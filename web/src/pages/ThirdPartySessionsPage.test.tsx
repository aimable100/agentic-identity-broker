import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ThirdPartySessionsPage } from './ThirdPartySessionsPage';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { consentApi } from '@services/api/consent';

vi.mock('@services/api/sessions', () => ({
  sessionsApi: { listSessions: vi.fn(), refreshSession: vi.fn(), getSessionDetails: vi.fn(), terminateSession: vi.fn() },
}));
vi.mock('@services/api/consent', () => ({
  consentApi: { getUserInfo: vi.fn(), getAgentDelegations: vi.fn() },
}));

const source = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="3"/></svg>';
const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>';
const session: SessionSummary = {
  id: 'session', service_id: 'atlas', service_display_name: 'Atlas Files',
  token_type: 'Bearer', scope: ['files.read'], initiated_at: '2026-11-18T10:00:00Z',
  requires_reauthentication: false, access_token_expired: false,
  has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};

describe('ThirdPartySessionsPage current icon metadata', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice@example.com', displayName: 'Alice' });
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);
  });

  it('keeps Refresh and Terminate mounted during refetch and carries fresh detail source to its identity header', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce([{ ...session, service_icon_svg: source }]);
    vi.mocked(sessionsApi.refreshSession).mockResolvedValue({ ...session, service_icon_svg: replacement });
    const { container } = render(<MemoryRouter><ThirdPartySessionsPage /></MemoryRouter>);
    const refresh = await screen.findByRole('button', { name: 'Refresh' });
    const terminate = screen.getByRole('button', { name: 'Terminate' });
    fireEvent.error(container.querySelector('img[src^="data:image/svg+xml"]')!);
    const pending = Promise.withResolvers<SessionSummary[]>();
    vi.mocked(sessionsApi.listSessions).mockReturnValueOnce(pending.promise);
    fireEvent.click(refresh);
    await waitFor(() => expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('button', { name: /refresh/i })).toBe(refresh);
    expect(screen.getByRole('button', { name: 'Terminate' })).toBe(terminate);
    await act(async () => pending.resolve([{ ...session, service_icon_svg: replacement }]));
    const recovered = container.querySelector('img[src^="data:image/svg+xml"]')!;
    expect(recovered).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
    fireEvent.load(recovered);
    expect(recovered).toBeVisible();
    expect(screen.getByRole('button', { name: 'Refresh' })).toBe(refresh);
    expect(screen.getByRole('button', { name: 'Terminate' })).toBe(terminate);
    vi.mocked(sessionsApi.getSessionDetails).mockResolvedValue({
      session: { ...session, service_icon_svg: replacement }, dependent_agents: [],
    });
    fireEvent.click(terminate);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('heading', { name: 'Atlas Files' })).toBeVisible();
    const detailImage = dialog.querySelector('img')!;
    expect(detailImage).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
    expect(detailImage).toHaveAttribute('alt', '');
    expect(dialog.querySelector('#termination-dialog-description img')).toBeNull();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel session termination' }));
    vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce([session]);
    fireEvent.click(refresh);
    await waitFor(() => expect(container.querySelector('img[src^="data:image/svg+xml"]')).toBeNull());
    expect(screen.getByRole('button', { name: 'Refresh' })).toBe(refresh);
    expect(screen.getByRole('heading', { name: 'Atlas Files' })).toBeVisible();
  });
});
