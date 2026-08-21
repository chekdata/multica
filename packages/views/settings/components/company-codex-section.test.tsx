import type { ReactNode } from "react";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { renderWithI18n } from "../../test/i18n";

const apiMock = vi.hoisted(() => ({
  createCompanyCodexKey: vi.fn(),
  getCompanyCodexKey: vi.fn(),
  increaseCompanyCodexQuota: vi.fn(),
  listCompanyCodexSessions: vi.fn(),
  revokeCompanyCodexKey: vi.fn(),
}));

vi.mock("@multica/core/api", () => ({ api: apiMock }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@multica/ui/components/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className} data-testid="company-codex-dialog">{children}</div>
  ),
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

import { CompanyCodexSection } from "./company-codex-section";

describe("CompanyCodexSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getCompanyCodexKey.mockResolvedValue({ active: false });
    apiMock.listCompanyCodexSessions.mockResolvedValue([]);
    apiMock.createCompanyCodexKey.mockResolvedValue({
      active: true,
      credential: `mck_${"x".repeat(240)}`,
      cc_switch_url: "ccswitch://providers/import?data=example",
      config_toml: "model_provider = \"chek\"",
      setup_command: "poolctl gui configure",
    });
  });

  it("contains long credentials and links to the official CC Switch releases", async () => {
    renderWithI18n(<CompanyCodexSection />);

    await waitFor(() => expect(screen.getByRole("button", { name: "Create access" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Create access" }));

    const dialog = await screen.findByTestId("company-codex-dialog");
    expect(dialog.className).toContain("overflow-x-hidden");
    expect(dialog.className).toContain("max-h-[calc(100dvh-2rem)]");

    const credential = dialog.querySelector("code");
    expect(credential?.className).toContain("overflow-hidden");
    expect(credential?.className).toContain("text-ellipsis");

    expect(screen.getByRole("link", { name: "Download CC Switch (official releases)" }))
      .toHaveAttribute("href", "https://github.com/farion1231/cc-switch/releases");
  });

  it("increases the existing key quota without rotating it", async () => {
    apiMock.getCompanyCodexKey.mockResolvedValue({
      active: true,
      key_prefix: "sk-clb-test",
      created_at: "2026-08-21T00:00:00Z",
      weekly_token_limit: 25_000_000,
      current_tokens: 5_000_000,
      reset_at: "2026-08-28T00:00:00Z",
      upstream_remaining_percent: 74.25,
    });
    apiMock.increaseCompanyCodexQuota.mockResolvedValue({
      active: true,
      key_prefix: "sk-clb-test",
      created_at: "2026-08-21T00:00:00Z",
      weekly_token_limit: 32_000_000,
      current_tokens: 5_000_000,
      reset_at: "2026-08-28T00:00:00Z",
      upstream_remaining_percent: 74.25,
    });

    renderWithI18n(<CompanyCodexSection />);

    await waitFor(() => expect(screen.getByRole("button", { name: "Increase quota" })).toBeEnabled());
    expect(screen.getByText(/5,000,000 \/ 25,000,000 tokens used this week/)).toBeInTheDocument();
    expect(screen.getByText(/Shared upstream pool remaining: 74.25%/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Increase quota" }));
    fireEvent.change(screen.getByLabelText("Additional tokens"), { target: { value: "7000000" } });
    expect(screen.getByText("25,000,000 + 7,000,000 = 32,000,000 tokens per week"))
      .toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirm increase" }));

    await waitFor(() => expect(apiMock.increaseCompanyCodexQuota).toHaveBeenCalledWith(7_000_000));
    expect(apiMock.createCompanyCodexKey).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText(/5,000,000 \/ 32,000,000 tokens used this week/))
      .toBeInTheDocument());
  });

  it("disables quota increases when the gateway quota is unavailable", async () => {
    apiMock.getCompanyCodexKey.mockResolvedValue({
      active: true,
      key_prefix: "sk-clb-test",
      created_at: "2026-08-21T00:00:00Z",
    });

    renderWithI18n(<CompanyCodexSection />);

    await waitFor(() => expect(screen.getByRole("button", { name: "Increase quota" }))
      .toBeDisabled());
    expect(screen.getByText("Quota status is temporarily unavailable. Refresh and try again."))
      .toBeInTheDocument();
  });
});
