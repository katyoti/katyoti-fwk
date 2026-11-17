// SPDX-License-Identifier: Apache-2.0
/**
 * MessageRenderer — universal message layout
 * ─────────────────────────────────────────────────────────────────────────
 * Owns: sender header (avatar + name + timestamp) + body indentation.
 * Dispatches to MessageWrapperBox (tool/system) or MessageWrapperRaw (text).
 */
import { Avatar, Box, Group, Text } from "@mantine/core";
import type { MsgItem } from "../../types/messages";
import { getSender } from "./senderRegistry";
import { getMessageRoute } from "./messageRegistry";
import { MessageWrapperBox } from "./MessageWrapperBox";
import { MessageWrapperRaw } from "./MessageWrapperRaw";
import { useT } from "../../i18n";
import { messageTime } from "./messageTime";

// Avatar 22px + gap 8px
const AVATAR_COL = 30;

interface Props {
  msg: MsgItem;
  isFirstInChain: boolean;
}

export function MessageRenderer({ msg, isFirstInChain }: Props) {
  const t = useT("chat");
  const route = getMessageRoute(msg, t);
  const sender = getSender(msg.sender, t);
  const { ContentRenderer, boxed } = route;

  const children = <ContentRenderer msg={msg} />;

  const content = boxed ? (
    <MessageWrapperBox route={route}>{children}</MessageWrapperBox>
  ) : (
    <MessageWrapperRaw>{children}</MessageWrapperRaw>
  );

  return (
    <Box>
      {/* ── Sender header (first in chain only) ──────────────── */}
      {isFirstInChain && (
        <Group gap={8} mb={2} px={2} align="flex-start" wrap="nowrap">
          <Avatar
            size={22}
            color={sender.color}
            radius="xl"
            style={{ fontSize: 9, flexShrink: 0, marginTop: 2 }}
          >
            <sender.Icon size={12} />
          </Avatar>
          <Group gap={6} align="baseline">
            <Text
              size="xs"
              fw={700}
              c={`${sender.color}.4`}
              style={{ lineHeight: 1.2 }}
            >
              {sender.name}
            </Text>
            <Text
              size="xs"
              c="dimmed"
              style={{ fontSize: 10, lineHeight: 1.2 }}
            >
              {messageTime(msg.at)}
            </Text>
          </Group>
        </Group>
      )}

      {/* ── Body — indented to align with text under avatar ──── */}
      <Box style={{ paddingLeft: AVATAR_COL }}>{content}</Box>
    </Box>
  );
}
