"use client";

import { useCallback, useEffect, useState } from "react";
import { Check, Copy, ExternalLink, KeyRound, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { api } from "@multica/core/api";
import type {
  CompanyCodexKeyStatus,
  CompanyCodexSession,
  CompanyCodexSessionDetail,
  CreateCompanyCodexKeyResponse,
} from "@multica/core/types";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { copyText } from "@multica/ui/lib/clipboard";
import { toast } from "sonner";
import { useT } from "../../i18n";
import { SettingsSection } from "./settings-layout";

type CopiedField = "credential" | "config" | "command" | null;

const CC_SWITCH_RELEASES_URL = "https://github.com/farion1231/cc-switch/releases";

export function CompanyCodexSection() {
  const { t } = useT("settings");
  const [keyStatus, setKeyStatus] = useState<CompanyCodexKeyStatus | null>(null);
  const [sessions, setSessions] = useState<CompanyCodexSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [issued, setIssued] = useState<CreateCompanyCodexKeyResponse | null>(null);
  const [copied, setCopied] = useState<CopiedField>(null);
  const [sessionDetail, setSessionDetail] = useState<CompanyCodexSessionDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [status, recentSessions] = await Promise.all([
        api.getCompanyCodexKey(),
        api.listCompanyCodexSessions(),
      ]);
      setKeyStatus(status);
      setSessions(recentSessions);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.company_codex.load_failed));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const issueKey = async () => {
    setCreating(true);
    try {
      const result = await api.createCompanyCodexKey();
      setIssued(result);
      setKeyStatus(result);
      toast.success(t(($) => $.company_codex.created));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.company_codex.create_failed));
    } finally {
      setCreating(false);
    }
  };

  const revokeKey = async () => {
    setRevoking(true);
    try {
      await api.revokeCompanyCodexKey();
      setKeyStatus({ active: false });
      toast.success(t(($) => $.company_codex.revoked));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.company_codex.revoke_failed));
    } finally {
      setRevoking(false);
    }
  };

  const copy = async (field: Exclude<CopiedField, null>, value: string) => {
    if (await copyText(value)) {
      setCopied(field);
      setTimeout(() => setCopied(null), 2000);
    }
  };

  const openSession = async (session: CompanyCodexSession) => {
    setDetailLoading(true);
    try {
      setSessionDetail(await api.getCompanyCodexSession(session.id));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.company_codex.session_load_failed));
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <>
      <SettingsSection
        title={t(($) => $.company_codex.title)}
        description={t(($) => $.company_codex.description)}
      >
        <Alert>
          <ShieldCheck />
          <AlertDescription>{t(($) => $.company_codex.audit_notice)}</AlertDescription>
        </Alert>

        {loading ? (
          <Card><CardContent><Skeleton className="h-10 w-full" /></CardContent></Card>
        ) : (
          <Card>
            <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-body font-medium">
                  <KeyRound className="h-4 w-4" />
                  {keyStatus?.active
                    ? t(($) => $.company_codex.active)
                    : t(($) => $.company_codex.inactive)}
                  <Badge variant={keyStatus?.active ? "secondary" : "outline"}>
                    {keyStatus?.active ? t(($) => $.company_codex.enabled) : t(($) => $.company_codex.not_enabled)}
                  </Badge>
                </div>
                {keyStatus?.active ? (
                  <p className="mt-1 text-caption text-muted-foreground">
                    {t(($) => $.company_codex.key_metadata, {
                      prefix: keyStatus.key_prefix ?? "",
                      date: keyStatus.created_at
                        ? new Date(keyStatus.created_at).toLocaleDateString()
                        : "",
                    })}
                  </p>
                ) : null}
              </div>
              <div className="flex shrink-0 gap-2">
                {keyStatus?.active ? (
                  <Button variant="outline" onClick={revokeKey} disabled={revoking}>
                    <Trash2 className="h-4 w-4" />
                    {t(($) => $.company_codex.revoke)}
                  </Button>
                ) : null}
                <Button onClick={issueKey} disabled={creating}>
                  <RefreshCw className={creating ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
                  {keyStatus?.active
                    ? t(($) => $.company_codex.rotate)
                    : t(($) => $.company_codex.create)}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.company_codex.sessions_title)}
        description={t(($) => $.company_codex.sessions_description)}
        action={
          <Button variant="ghost" size="sm" onClick={() => void refresh()}>
            <RefreshCw className="h-3.5 w-3.5" />
            {t(($) => $.company_codex.refresh)}
          </Button>
        }
      >
        {sessions.length === 0 ? (
          <Card><CardContent className="text-caption text-muted-foreground">{t(($) => $.company_codex.sessions_empty)}</CardContent></Card>
        ) : (
          <div className="space-y-2">
            {sessions.map((session) => (
              <button
                type="button"
                key={session.id}
                onClick={() => void openSession(session)}
                className="w-full rounded-lg border border-surface-border bg-surface px-4 py-3 text-left transition-colors hover:bg-surface-hover"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="truncate text-body font-medium">{session.title}</div>
                    <div className="mt-1 truncate text-caption text-muted-foreground">
                      {session.user_name} · {session.model || t(($) => $.company_codex.unknown_model)} · {new Date(session.last_activity_at).toLocaleString()}
                    </div>
                    {session.last_prompt ? (
                      <p className="mt-2 line-clamp-2 text-caption text-muted-foreground">{session.last_prompt}</p>
                    ) : null}
                  </div>
                  <Badge variant="outline" className="shrink-0">
                    {t(($) => $.company_codex.token_count, {
                      count: session.input_tokens + session.output_tokens,
                    })}
                  </Badge>
                </div>
              </button>
            ))}
          </div>
        )}
      </SettingsSection>

      <Dialog open={!!issued} onOpenChange={(open) => { if (!open) setIssued(null); }}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] min-w-0 overflow-x-hidden overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t(($) => $.company_codex.created_title)}</DialogTitle>
          </DialogHeader>
          <Alert>
            <ShieldCheck />
            <AlertDescription>{t(($) => $.company_codex.created_warning)}</AlertDescription>
          </Alert>
          {issued ? (
            <div className="min-w-0 space-y-4">
              <div className="min-w-0 space-y-1.5">
                <div className="text-caption font-medium">{t(($) => $.company_codex.credential)}</div>
                <div className="flex min-w-0 gap-2">
                  <code className="block min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap rounded-md border bg-muted/50 px-3 py-2 text-body select-all">{issued.credential}</code>
                  <Button variant="outline" size="icon" onClick={() => void copy("credential", issued.credential)}>
                    {copied === "credential" ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <Card className="min-w-0">
                  <CardContent className="space-y-3">
                    <div>
                      <div className="text-body font-medium">{t(($) => $.company_codex.cc_switch)}</div>
                      <p className="mt-1 text-caption text-muted-foreground">{t(($) => $.company_codex.cc_switch_hint)}</p>
                    </div>
                    <Button className="w-full min-w-0" onClick={() => window.location.assign(issued.cc_switch_url)}>
                      <ExternalLink className="h-4 w-4" />
                      <span className="truncate">{t(($) => $.company_codex.open_cc_switch)}</span>
                    </Button>
                    <a
                      href={CC_SWITCH_RELEASES_URL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-caption text-muted-foreground underline underline-offset-4 hover:text-foreground"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                      {t(($) => $.company_codex.download_cc_switch)}
                    </a>
                  </CardContent>
                </Card>
                <Card className="min-w-0">
                  <CardContent className="space-y-3">
                    <div>
                      <div className="text-body font-medium">{t(($) => $.company_codex.official_gui)}</div>
                      <p className="mt-1 text-caption text-muted-foreground">{t(($) => $.company_codex.official_gui_hint)}</p>
                    </div>
                    <Button variant="outline" className="w-full min-w-0" onClick={() => void copy("command", issued.setup_command)}>
                      {copied === "command" ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                      <span className="truncate">{t(($) => $.company_codex.copy_command)}</span>
                    </Button>
                  </CardContent>
                </Card>
              </div>

              <details className="min-w-0 rounded-lg border border-surface-border px-4 py-3">
                <summary className="cursor-pointer text-body font-medium">{t(($) => $.company_codex.manual_config)}</summary>
                <pre className="mt-3 max-h-56 overflow-auto whitespace-pre-wrap rounded-md bg-muted/50 p-3 text-caption">{issued.config_toml}</pre>
                <Button variant="ghost" size="sm" className="mt-2" onClick={() => void copy("config", issued.config_toml)}>
                  {copied === "config" ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  {t(($) => $.company_codex.copy_config)}
                </Button>
              </details>
            </div>
          ) : null}
          <DialogFooter>
            <Button onClick={() => setIssued(null)}>{t(($) => $.company_codex.done)}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!sessionDetail || detailLoading} onOpenChange={(open) => { if (!open) setSessionDetail(null); }}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{sessionDetail?.session.title ?? t(($) => $.company_codex.loading_session)}</DialogTitle>
          </DialogHeader>
          {detailLoading && !sessionDetail ? <Skeleton className="h-40 w-full" /> : null}
          {sessionDetail ? (
            <div className="space-y-4">
              <p className="text-caption text-muted-foreground">
                {sessionDetail.session.user_name} · {sessionDetail.session.model || t(($) => $.company_codex.unknown_model)} · {t(($) => $.company_codex.token_count, { count: sessionDetail.session.input_tokens + sessionDetail.session.output_tokens })}
              </p>
              {sessionDetail.turns.map((turn) => (
                <div key={turn.id} className="space-y-2 rounded-lg border border-surface-border p-4">
                  <div className="text-caption font-medium">{t(($) => $.company_codex.employee_prompt)}</div>
                  <pre className="whitespace-pre-wrap break-words text-body font-sans">{turn.prompt}</pre>
                  <div className="pt-2 text-caption font-medium">{t(($) => $.company_codex.codex_response)}</div>
                  <pre className="whitespace-pre-wrap break-words text-body font-sans">{turn.response}</pre>
                  <div className="text-micro text-muted-foreground">{new Date(turn.completed_at).toLocaleString()} · {t(($) => $.company_codex.token_count, { count: turn.input_tokens + turn.output_tokens })}</div>
                </div>
              ))}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
