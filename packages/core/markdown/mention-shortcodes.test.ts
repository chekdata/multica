// @vitest-environment node
import { expect, it } from "vitest";
import { preprocessMentionShortcodes } from "./mention-shortcodes";

it("preserves legacy mention attributes across whitespace", () => {
  expect(preprocessMentionShortcodes('[@   id="user-1" label="Alex"]')).toBe('[@Alex](mention://member/user-1)');
});
it("preserves an unterminated mention with a long whitespace run", () => {
  const text = "[@ " + " ".repeat(100000);
  expect(preprocessMentionShortcodes(text)).toBe(text);
});
