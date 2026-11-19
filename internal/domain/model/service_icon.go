package model

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const iconXMLWhitespace = " \t\r\n"

var (
	errIconDocument = errors.New("icon_svg must be a well-formed SVG document")
	errIconContent  = errors.New("icon_svg contains prohibited content")
)

func ValidateServiceIcon(source *string) error {
	if source == nil {
		return nil
	}
	if len(*source) == 0 || len(*source) > 65536 || !utf8.ValidString(*source) {
		return errors.New("icon_svg must contain 1 to 65536 UTF-8 bytes")
	}
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimPrefix(*source, "\ufeff")))
	depth := 0
	rootSeen, doctypeSeen := false, false
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			if !rootSeen || depth != 0 {
				return errIconDocument
			}
			return nil
		}
		if err != nil {
			return errIconDocument
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if rootSeen || token.Name.Local != "svg" || token.Name.Space != "http://www.w3.org/2000/svg" {
					return errIconDocument
				}
				rootSeen = true
			}
			if err := validateIconElement(token); err != nil {
				return err
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.Trim(token, iconXMLWhitespace)) != 0 {
				return errIconDocument
			}
		case xml.Directive:
			if rootSeen || doctypeSeen || !validIconDoctype(string(token)) {
				return errIconContent
			}
			doctypeSeen = true
		case xml.ProcInst:
			if offset != 0 || token.Target != "xml" || !validIconDeclaration(token.Inst) {
				return errIconContent
			}
		}
	}
}

func validateIconElement(element xml.StartElement) error {
	for _, name := range []string{"script", "a", "foreignobject"} {
		if iconASCIIEqual(element.Name.Local, name) {
			return errIconContent
		}
	}
	seen := make(map[xml.Name]struct{}, len(element.Attr))
	for _, attr := range element.Attr {
		if _, duplicate := seen[attr.Name]; duplicate {
			return errIconDocument
		}
		seen[attr.Name] = struct{}{}
		name := attr.Name.Local
		if len(name) >= 2 && name[0]|0x20 == 'o' && name[1]|0x20 == 'n' || iconScriptURL(attr.Value) {
			return errIconContent
		}
	}
	return nil
}

func iconASCIIEqual(value, lower string) bool {
	if len(value) != len(lower) {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

func iconScriptURL(value string) bool {
	scheme, matched := "", 0
	for i := range len(value) {
		c := value[i]
		if c <= ' ' || c == 0x7f {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if matched == 0 {
			switch c {
			case 'j':
				scheme = "javascript:"
			case 'v':
				scheme = "vbscript:"
			default:
				return false
			}
		}
		if c != scheme[matched] {
			return false
		}
		matched++
		if matched == len(scheme) {
			return true
		}
	}
	return false
}

func iconXMLWord(value string) (string, string) {
	value = strings.TrimLeft(value, iconXMLWhitespace)
	if end := strings.IndexAny(value, iconXMLWhitespace); end >= 0 {
		return value[:end], value[end:]
	}
	return value, ""
}

func iconQuotedLiteral(value string) (string, bool) {
	trimmed := strings.TrimLeft(value, iconXMLWhitespace)
	if len(trimmed) == len(value) || len(trimmed) < 2 || trimmed[0] != '\'' && trimmed[0] != '"' {
		return "", false
	}
	end := strings.IndexByte(trimmed[1:], trimmed[0])
	if end < 0 {
		return "", false
	}
	return trimmed[end+2:], true
}

func validIconDoctype(value string) bool {
	word, rest := iconXMLWord(value)
	if word != "DOCTYPE" {
		return false
	}
	name, rest := iconXMLWord(rest)
	if name != "svg" && !strings.HasSuffix(name, ":svg") {
		return false
	}
	if strings.Trim(rest, iconXMLWhitespace) == "" {
		return true
	}
	kind, rest := iconXMLWord(rest)
	if kind != "SYSTEM" && kind != "PUBLIC" {
		return false
	}
	rest, ok := iconQuotedLiteral(rest)
	if !ok {
		return false
	}
	if kind == "PUBLIC" {
		rest, ok = iconQuotedLiteral(rest)
	}
	return ok && strings.Trim(rest, iconXMLWhitespace) == ""
}

func validIconDeclaration(instruction []byte) bool {
	decoder := xml.NewDecoder(io.MultiReader(strings.NewReader("<declaration "), bytes.NewReader(instruction), strings.NewReader("/>")))
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	element, ok := token.(xml.StartElement)
	if !ok || len(element.Attr) == 0 || element.Attr[0].Name != (xml.Name{Local: "version"}) || element.Attr[0].Value != "1.0" {
		return false
	}
	last := 0
	for _, attr := range element.Attr[1:] {
		switch {
		case last == 0 && attr.Name == (xml.Name{Local: "encoding"}) && iconASCIIEqual(attr.Value, "utf-8"):
			last = 1
		case last < 2 && attr.Name == (xml.Name{Local: "standalone"}) && (attr.Value == "yes" || attr.Value == "no"):
			last = 2
		default:
			return false
		}
	}
	if _, err := decoder.Token(); err != nil {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}
