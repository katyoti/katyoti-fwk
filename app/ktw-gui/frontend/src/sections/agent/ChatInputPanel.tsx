// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import {
  Badge,
  Button,
  Divider,
  Group,
  Paper,
  ScrollArea,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { useAppStore, useSessionPrompt } from "../../store";
import { AnswerPrompt } from "../../../wailsjs/go/main/App";
import { ChatInput } from "./ChatInput";
import { DynamicForm } from "../../components/forms/DynamicForm";
import { Markdown } from "../../components/Markdown";
import { useInboxLauncher } from "../../hooks/useInboxLauncher";
import { useT } from "../../i18n";

/**
 * Input panel:
 *  - When a prompt is active: shows an inline prompt banner above ChatInput
 *  - Otherwise: just ChatInput
 */
export function ChatInputPanel({ sessionId }: { sessionId?: string } = {}) {
  const [text, setText] = useState("");
  const [textAnswer, setTextAnswer] = useState("");
  const t = useT("chat");

  // THIS session's pending prompt — explicit per-run targeting. Answers are
  // echoed into + routed to this session, not whatever is focused.
  const activePrompt = useSessionPrompt(sessionId);
  const setPrompt    = useAppStore((s) => s.setPrompt);
  const addMessage   = useAppStore((s) => s.addMessage);
  const answer = (id: string, value: string) => AnswerPrompt(sessionId ?? "", id, value);
  const { launchInbox, canLaunch: inboxCanLaunch } = useInboxLauncher();

  // Echo the user's prompt answer into the chat thread BEFORE we
  // hand it off via AnswerPrompt. The translator's onUserInput hook
  // only updates the prompt-banner's `answered` flag — it does NOT
  // emit a separate user-role message, so without this echo the
  // user's typed text vanishes from the conversation history.
  // Display-only; the canonical answer flows through HumanGate as
  // before.
  function echoUserAnswer(ans: string) {
    if (!activePrompt) return;
    addMessage({
      kind:   "text",
      id:     `user-ans-${activePrompt.id}`,
      sender: "user",
      text:   ans,
      done:   true,
    }, sessionId);
  }

  async function handleTextAnswer() {
    if (!activePrompt || !textAnswer.trim()) return;
    const ans = textAnswer.trim();
    setTextAnswer("");
    echoUserAnswer(ans);
    setPrompt(null, sessionId);
    await answer(activePrompt.id,ans);
  }

  async function handleSelect(opt: string) {
    if (!activePrompt) return;
    echoUserAnswer(opt);
    setPrompt(null, sessionId);
    await answer(activePrompt.id,opt);
  }

  async function handleConfirm(ans: string) {
    if (!activePrompt) return;
    echoUserAnswer(ans);
    setPrompt(null, sessionId);
    await answer(activePrompt.id,ans);
  }

  return (
    <Stack
      gap={0}
      style={{
        flexShrink: 0,
        padding: "8px 12px 10px",
      }}
    >
      {/* ── Inbox-routed ask: notice instead of input ─────────────
          The question went to the workspace Inbox (the task is
          suspended on the item, not on the HumanGate) — answering
          here would be a dead end. Point the user at the Inbox. */}
      {activePrompt && activePrompt.channel === "inbox" && (
        <Paper
          withBorder
          p="sm"
          radius="sm"
          style={{
            background: "var(--mantine-color-default)",
            borderColor: "var(--mantine-color-yellow-7)",
          }}
        >
          {/* The question is markdown (lists, tables, emphasis): rendered,
              in a bounded scroll area so a long one never covers the chat. */}
          <Stack gap="xs">
            <Group gap="xs" align="center" justify="space-between" wrap="nowrap">
              <Group gap="xs" align="center" wrap="nowrap">
                <Badge size="xs" color="yellow" variant="light">
                  {t("prompt.inboxBadge")}
                </Badge>
                <Text size="xs" c="dimmed">
                  {t("prompt.inboxNotice")}
                </Text>
              </Group>
              <Button
                size="compact-xs"
                variant="light"
                color="yellow"
                onClick={() => launchInbox()}
                disabled={!inboxCanLaunch}
              >
                {t("prompt.openInbox")}
              </Button>
            </Group>
            <ScrollArea.Autosize mah="40vh" type="auto" offsetScrollbars>
              <Markdown text={activePrompt.prompt ?? ""} />
            </ScrollArea.Autosize>
          </Stack>
        </Paper>
      )}

      {/* ask-user-data forms render as interactive cards in the chat
          THREAD (see useAgentEvents → 'ask-form' message), not here. */}

      {/* ── Inline prompt banner ──────────────────────────────── */}
      {activePrompt && activePrompt.channel !== "inbox" && activePrompt.channel !== "form" && (
        <>
          <Paper
            withBorder
            p="sm"
            radius="sm"
            style={{
              background: "var(--mantine-color-default)",
              borderColor: "var(--mantine-color-blue-7)",
            }}
          >
            <Stack gap="xs">
              <Group gap="xs" align="center">
                <Badge size="xs" color="blue" variant="light">
                  {t("prompt.promptBadge")}
                </Badge>
                <Text size="sm" c="gray.2">
                  {activePrompt.prompt}
                </Text>
              </Group>

              {activePrompt.type === "text" && (
                <Group gap="xs">
                  <TextInput
                    size="xs"
                    placeholder={
                      activePrompt.placeholder ?? t("prompt.answerPlaceholder")
                    }
                    value={textAnswer}
                    onChange={(e) => setTextAnswer(e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") handleTextAnswer();
                    }}
                    style={{ flex: 1 }}
                    autoFocus
                  />
                  <Button size="xs" onClick={handleTextAnswer}>
                    {t("prompt.send")}
                  </Button>
                </Group>
              )}

              {activePrompt.type === "select" && (
                <Group gap="xs" wrap="wrap">
                  {(activePrompt.options ?? []).map((opt) => (
                    <Button
                      key={opt}
                      size="xs"
                      variant="outline"
                      onClick={() => handleSelect(opt)}
                    >
                      {opt}
                    </Button>
                  ))}
                </Group>
              )}

              {activePrompt.type === "confirm" && (
                <Group gap="xs">
                  <Button
                    size="xs"
                    color="green"
                    onClick={() => handleConfirm("yes")}
                  >
                    {t("prompt.yes")}
                  </Button>
                  <Button
                    size="xs"
                    color="red"
                    variant="outline"
                    onClick={() => handleConfirm("no")}
                  >
                    {t("prompt.no")}
                  </Button>
                </Group>
              )}
            </Stack>
          </Paper>

          {activePrompt.type === "form" && activePrompt.formSchema && (
            <Paper
              withBorder
              radius="sm"
              mt="xs"
              style={{
                background: "var(--mantine-color-default)",
                borderColor: "var(--mantine-color-blue-7)",
                maxHeight: 500,
                overflow: 'auto',
              }}
            >
              <DynamicForm
                schema={activePrompt.formSchema}
                onSubmit={async (data) => {
                  echoUserAnswer(JSON.stringify(data));
                  setPrompt(null, sessionId);
                  await answer(activePrompt.id,JSON.stringify(data));
                }}
                onCancel={() => {
                  setPrompt(null, sessionId);
                  answer(activePrompt.id,"");
                }}
              />
            </Paper>
          )}
        </>
      )}

      {/* ── Main chat input ───────────────────────────────────── */}
      {!activePrompt && <ChatInput text={text} onChange={setText} sessionId={sessionId} />}
    </Stack>
  );
}
