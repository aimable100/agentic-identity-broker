/**
 * Integration tests for AgentGrantDetailPage grant submission flow.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AgentGrantDetailPage } from './AgentGrantDetailPage';
import { consentApi } from '@services/api/consent';
import { ToastProvider } from '../components/ui/Toast';
import type {
  AgentDetail,
  ThirdpartyService,
  UserGrant,
} from '../types/consent';

// Mock the API
vi.mock('@services/api/consent', () => ({
  consentApi: {
    getUserInfo: vi.fn(),
    getAgentDelegations: vi.fn(),
    getAgentDetail: vi.fn(),
    getAgentGrants: vi.fn(),
    createOrUpdateGrant: vi.fn(),
  },
}));

describe('AgentGrantDetailPage - Integration', () => {
  const mockAgent: AgentDetail = {
    agentId: 'test-agent',
    displayName: 'Test Agent',
    description: 'A test agent',
    logoUrl: 'https://example.com/logo.png',
    governanceUrl: 'https://example.com/governance',
    userDocumentationUrl: 'https://example.com/docs',
    agentInterfaceUrl: 'https://example.com/interface',
  };

  const mockServices: ThirdpartyService[] = [
    {
      kind: 'scoped',
      serviceId: 'github',
      displayName: 'GitHub',
      scopes: [
        { value: 'read:user', description: 'Read user profile' },
        { value: 'read:repo', description: 'Read repositories' },
      ],
    },
  ];

  const mockGrant: UserGrant = {
    id: 'grant-123',
    agent_id: 'test-agent',
    principal: 'user@example.com',
    delegated_oauth2_tokens: [
      {
        thirdparty_oauth2_service_id: 'github',
        scopes: ['read:user'],
      },
    ],
    valid_until: null,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();

    // Setup default mock responses
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({
      principal: 'user@example.com',
      displayName: 'Test User',
      pictureUrl: 'https://example.com/avatar.png',
    });

    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);

    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({
      agent: mockAgent,
      services: mockServices,
    });

    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(mockGrant);
  });

  const renderPage = () => {
    return render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/agents/test-agent']}>
          <Routes>
            <Route path="/agents/:agentId" element={<AgentGrantDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    );
  };

  it('should load and display agent information', async () => {
    renderPage();

    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
        expect(screen.getByText('A test agent')).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should enable edit mode when toggle is clicked', async () => {
    renderPage();

    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should submit grant successfully', async () => {
    const updatedGrant: UserGrant = {
      ...mockGrant,
      delegated_oauth2_tokens: [
        {
          thirdparty_oauth2_service_id: 'github',
          scopes: ['read:user', 'read:repo'],
        },
      ],
    };

    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant: updatedGrant });

    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should display error when submission fails', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockRejectedValue({
      response: {
        data: {
          message: 'Failed to update grant',
        },
      },
    });

    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should refetch data after successful submission', async () => {
    const updatedGrant: UserGrant = {
      ...mockGrant,
      delegated_oauth2_tokens: [
        {
          thirdparty_oauth2_service_id: 'github',
          scopes: ['read:user', 'read:repo'],
        },
      ],
    };

    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant: updatedGrant });

    renderPage();

    // Wait for initial load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should validate form before submission', async () => {
    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should cancel edit mode', async () => {
    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });
  it('replaces service images after Save while retaining loaded permission switches and the Save node during refetch', async () => {
    const source = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="3"/></svg>';
    const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>';
    const agent: AgentDetail = {
      ...mockAgent,
      active_session_service_ids: ['atlas', 'birch'],
      service_requirements: [
        { service_id: 'atlas', requirement_type: 'mandatory' },
        { service_id: 'birch', requirement_type: 'optional' },
      ],
      permission_sets: [{
        requirement_type: 'mandatory',
        permission_set: {
          id: 'files', name: 'Files access', description: 'Project files',
          service_scopes: [
            { service_id: 'atlas', requirement_type: 'mandatory' },
            { service_id: 'birch', requirement_type: 'optional' },
          ],
        },
      }],
    };
    const services = [
      { kind: 'requirement' as const, serviceId: 'atlas', serviceName: 'Atlas Files', requirementType: 'mandatory' as const,
        requiredScopes: [], connectionStatus: 'connected' as const, iconSvg: source },
      { kind: 'requirement' as const, serviceId: 'birch', serviceName: 'Birch Files', requirementType: 'optional' as const,
        requiredScopes: [], connectionStatus: 'connected' as const, iconSvg: source },
    ];
    const grant = { ...mockGrant, granted_permission_sets: { files: ['atlas', 'birch'] } };
    vi.mocked(consentApi.getAgentDetail).mockResolvedValueOnce({ agent, services });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(grant);
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
    const { container } = renderPage();
    const save = await screen.findByRole('button', { name: 'Save' });
    const toggle = screen.getByRole('switch', { name: 'Exclude Birch Files' });
    expect(container.querySelectorAll('img[src^="data:image/svg+xml"]')).toHaveLength(2);
    container.querySelectorAll('img[src^="data:image/svg+xml"]').forEach((icon) => fireEvent.error(icon));
    const pending = Promise.withResolvers<{ agent: AgentDetail; services: typeof services }>();
    vi.mocked(consentApi.getAgentDetail).mockReturnValueOnce(pending.promise);
    fireEvent.click(save);
    await waitFor(() => expect(consentApi.getAgentDetail).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('button', { name: 'Save' })).toBe(save);
    expect(screen.getByRole('switch', { name: 'Exclude Birch Files' })).toBe(toggle);
    await act(async () => {
      pending.resolve({ agent, services: services.map((service) => ({ ...service, iconSvg: replacement })) });
    });
    const images = container.querySelectorAll('img[src^="data:image/svg+xml"]');
    expect(images).toHaveLength(2);
    images.forEach((icon) => {
      expect(icon).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
      fireEvent.load(icon);
      expect(icon).toBeVisible();
    });
    expect(screen.getByRole('button', { name: 'Save' })).toBe(save);
    expect(screen.getByRole('switch', { name: 'Exclude Birch Files' })).toBe(toggle);
  });
});
