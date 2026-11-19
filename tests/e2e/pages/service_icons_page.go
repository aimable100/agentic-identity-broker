package pages

import (
	"encoding/json"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

// ServiceIconSurface identifies the existing service identity, not a new screen.
type ServiceIconSurface string

const (
	ServiceIconConnection ServiceIconSurface = "connection"
	ServiceIconRequired   ServiceIconSurface = "required-permission"
	ServiceIconOptional   ServiceIconSurface = "optional-permission"
	ServiceIconSession    ServiceIconSurface = "session"
	ServiceIconDetail     ServiceIconSurface = "session-detail"
)

type ServiceIconImage struct {
	Source       string `json:"source"`
	Alt          string `json:"alt"`
	Complete     bool   `json:"complete"`
	NaturalWidth int    `json:"naturalWidth"`
	Focusable    bool   `json:"focusable"`
}

type ServiceIconIdentity struct {
	NameVisible bool               `json:"nameVisible"`
	Initials    int                `json:"initials"`
	Images      []ServiceIconImage `json:"images"`
	InlineArt   int                `json:"inlineArt"`
	Embeddings  int                `json:"embeddings"`
}

// ServiceIconsPage adds icon observations to the maintained consent/session pages.
// It deliberately keeps all identity selectors out of acceptance scenarios.
type ServiceIconsPage struct {
	*Page
}

func NewServiceIconsPage(page playwright.Page, baseURL string) *ServiceIconsPage {
	return &ServiceIconsPage{Page: NewPage(page, baseURL)}
}

func (p *ServiceIconsPage) identity(surface ServiceIconSurface, serviceID, name string) playwright.Locator {
	page := p.GetPlaywrightPage()
	switch surface {
	case ServiceIconConnection:
		return page.GetByTestId("service-card-" + serviceID)
	case ServiceIconRequired:
		return page.GetByLabel(name+" (required)", playwright.PageGetByLabelOptions{Exact: playwright.Bool(true)})
	case ServiceIconOptional:
		return page.Locator("[data-testid^='permission-set-']").GetByText(name, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).Locator("xpath=..")
	case ServiceIconSession:
		return (&SessionsPage{Page: p.Page}).sessionCard(name)
	case ServiceIconDetail:
		return (&SessionsPage{Page: p.Page}).sessionDetails()
	default:
		panic("unknown service icon surface: " + string(surface))
	}
}

func (p *ServiceIconsPage) ReadIdentity(surface ServiceIconSurface, serviceID, name string) (ServiceIconIdentity, error) {
	var result ServiceIconIdentity
	root := p.identity(surface, serviceID, name)
	if err := root.WaitFor(); err != nil {
		return result, err
	}
	value, err := root.Evaluate(`(root, name) => {
		const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
		let text;
		while (walker.nextNode()) {
			if (walker.currentNode.textContent.trim() === name) { text = walker.currentNode; break; }
		}
		const range = document.createRange();
		if (text) range.selectNodeContents(text);
		const rect = text && range.getBoundingClientRect();
		const style = text && getComputedStyle(text.parentElement);
		return {
			nameVisible: !!rect && rect.width > 0 && rect.height > 0 && rect.left >= 0 && rect.right <= innerWidth && style.visibility !== 'hidden',
			initials: [...root.querySelectorAll('span')].filter(el => el.textContent === name[0].toUpperCase() && el.getBoundingClientRect().width > 0).length,
			images: [...root.querySelectorAll('img')].map(img => ({source: img.getAttribute('src'), alt: img.alt, complete: img.complete, naturalWidth: img.naturalWidth, focusable: img.tabIndex >= 0 || !!img.closest('a,button,[tabindex="0"]')})),
			inlineArt: root.querySelectorAll('[id="service-icon-artwork"], svg metadata').length,
			embeddings: root.querySelectorAll('object,embed,iframe').length
		};
	}`, name)
	if err != nil {
		return result, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(encoded, &result)
	return result, err
}

func (p *ServiceIconsPage) WaitForImage(surface ServiceIconSurface, serviceID, name, source string) error {
	img := p.identity(surface, serviceID, name).Locator("img")
	if err := img.WaitFor(); err != nil {
		return err
	}
	return img.WaitForFunction(`(img, source) => img.complete && img.naturalWidth > 0 && img.getAttribute('src')?.startsWith('data:image/svg+xml;charset=utf-8,') && decodeURIComponent(img.getAttribute('src').split(',').slice(1).join(',')) === source`, source)
}

// FailImage dispatches the native error event against a real, successfully decoded
// image backed by valid stored source. It deterministically exercises the same
// React onError path without weakening validation or relying on browser quirks.
func (p *ServiceIconsPage) FailImage(surface ServiceIconSurface, serviceID, name, source string) error {
	if err := p.WaitForImage(surface, serviceID, name, source); err != nil {
		return err
	}
	return p.identity(surface, serviceID, name).Locator("img").DispatchEvent("error", nil)
}

// RememberControls retains actual DOM nodes and the document identity. A changed
// image must not recover by replacing its surrounding action controls or page.
func (p *ServiceIconsPage) RememberControls() error {
	_, err := p.GetPlaywrightPage().Evaluate(`() => {
		window.__serviceIconControls = [...document.querySelectorAll('button,input')].filter(el => !el.closest('[role="dialog"]'));
		window.__serviceIconDocument = document;
	}`)
	return err
}

func (p *ServiceIconsPage) ControlsPreserved() (bool, error) {
	value, err := p.GetPlaywrightPage().Evaluate(`() => window.__serviceIconDocument === document && window.__serviceIconControls.length > 0 && window.__serviceIconControls.every(el => el.isConnected)`)
	if err != nil {
		return false, err
	}
	return value == true, nil
}

// KeyboardActivate reaches an existing control by real Tab navigation; it fails
// if a decorative image introduces a tab stop along the way.
func (p *ServiceIconsPage) KeyboardActivate(surface ServiceIconSurface, serviceID, name, action string) error {
	var button playwright.Locator
	switch surface {
	case ServiceIconRequired, ServiceIconOptional:
		button = p.identity(surface, serviceID, name).GetByRole("switch")
	case ServiceIconDetail:
		button = (&SessionsPage{Page: p.Page}).sessionDetailsAction(action)
	default:
		button = p.identity(surface, serviceID, name).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: action, Exact: playwright.Bool(true)})
	}
	return p.keyboardActivate(button)
}

func (p *ServiceIconsPage) KeyboardSaveConsent() error {
	return p.keyboardActivate((&ConsentPage{Page: p.Page}).getConsentActionButton("Save"))
}

func (p *ServiceIconsPage) keyboardActivate(button playwright.Locator) error {
	if err := button.WaitFor(); err != nil {
		return err
	}
	for range 100 {
		if err := p.GetPlaywrightPage().Keyboard().Press("Tab"); err != nil {
			return err
		}
		badFocus, err := p.GetPlaywrightPage().Evaluate(`() => document.activeElement?.tagName === 'IMG'`)
		if err != nil {
			return err
		}
		if badFocus == true {
			return fmt.Errorf("decorative service image received keyboard focus")
		}
		focused, err := button.Evaluate(`el => el === document.activeElement`, nil)
		if err != nil {
			return err
		}
		if focused == true {
			return button.Press("Enter")
		}
	}
	return fmt.Errorf("keyboard could not reach the requested service control")
}

func (p *ServiceIconsPage) IdentityAccessibility(surface ServiceIconSurface, serviceID, name string) (string, error) {
	return p.identity(surface, serviceID, name).AriaSnapshot()
}

func (p *ServiceIconsPage) IconAccessibility(surface ServiceIconSurface, serviceID, name string) (string, error) {
	icons := p.identity(surface, serviceID, name).Locator("img[src^='data:image/svg+xml;']")
	count, err := icons.Count()
	if err != nil || count == 0 {
		return "", err
	}
	return icons.AriaSnapshot()
}

func (p *ServiceIconsPage) HasPageError() (bool, error) {
	return p.GetPlaywrightPage().GetByRole("alert").IsVisible()
}

func (p *ServiceIconsPage) WaitForRemoteAvatar(url string) error {
	img := p.GetPlaywrightPage().Locator("img").Filter(playwright.LocatorFilterOptions{Visible: playwright.Bool(true)})
	all, err := img.All()
	if err != nil {
		return err
	}
	for _, candidate := range all {
		src, err := candidate.GetAttribute("src")
		if err != nil {
			return err
		}
		if src == url {
			return candidate.WaitForFunction(`img => img.complete && img.naturalWidth > 0`, nil)
		}
	}
	return fmt.Errorf("remote user avatar %s is not rendered", url)
}

// ReopenConnectionsAfterExpiry uses SPA navigation, retaining the actual API
// cache until its supported five-minute TTL expires. It is not recovery proof.
func (p *ServiceIconsPage) ReopenConnectionsAfterExpiry(agentName string) error {
	page := p.GetPlaywrightPage()
	now, err := page.Evaluate(`() => Date.now() + 5 * 60 * 1000 + 1`)
	if err != nil {
		return err
	}
	if err := page.Clock().SetFixedTime(now); err != nil {
		return err
	}
	// The existing desktop navigation is hidden at mobile widths.
	viewport := page.ViewportSize()
	if err := page.SetViewportSize(1280, 900); err != nil {
		return err
	}
	if err := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Agent Delegations", Exact: playwright.Bool(true)}).Click(); err != nil {
		return err
	}
	if err := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: agentName, Exact: playwright.Bool(true)}).Click(); err != nil {
		return err
	}
	return page.SetViewportSize(viewport.Width, viewport.Height)
}
