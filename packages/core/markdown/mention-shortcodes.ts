/**
 * Convert legacy mention shortcodes [@ id="UUID" label="LABEL"] to the
 * standard markdown link format [@LABEL](mention://member/UUID).
 *
 * These shortcodes exist in older database records from a previous mention
 * serialization format. This function normalises them so downstream parsers
 * (Tiptap @tiptap/markdown on web/desktop, the mobile renderer in
 * apps/mobile/lib/markdown/) only need to handle one syntax.
 *
 * Single source of truth for all clients. Mobile imports this directly
 * because mobile is allowed to import pure functions from @multica/core/.
 * Web/desktop continue to access it via @multica/ui/markdown which now
 * re-exports from here, so existing import paths keep working.
 *
 * Pure regex transform — no IO, no global state. Idempotent: running it
 * twice on the same input produces the same output.
 */
export function preprocessMentionShortcodes(text: string): string {
  if (!text.includes("[@ ")) return text;
  const parts: string[] = [];
  let cursor = 0;
  let copied = 0;
  while (cursor < text.length) {
    const start = text.indexOf("[@", cursor);
    if (start < 0) break;
    cursor = start + 2;
    if (!/\s/.test(text[cursor] ?? "")) continue;
    const end = text.indexOf("]", cursor + 1);
    if (end < 0) break;
    const attrs: Record<string, string> = {};
    const re = /\b(\w+)="([^"]*)"/g;
    const attrString = text.slice(cursor + 1, end);
    let match;
    while ((match = re.exec(attrString)) !== null) {
      if (match[1] && match[2] !== undefined) attrs[match[1]] = match[2];
    }
    if (attrs.id && attrs.label) {
      parts.push(text.slice(copied, start), `[@${attrs.label}](mention://member/${attrs.id})`);
      copied = end + 1;
    }
    cursor = end + 1;
  }
  parts.push(text.slice(copied));
  return parts.join("");
}
