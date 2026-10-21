/**
 * Convert legacy mention shortcodes [@ id="UUID" label="LABEL"] to the
 * standard markdown link format [@LABEL](mention://member/UUID).
 *
 * These shortcodes exist in older database records from a previous mention
 * serialization format. This function normalises them so downstream parsers
 * (Tiptap @tiptap/markdown, react-markdown) only need to handle one syntax.
 *
 * SYNCED COPY — KEEP IDENTICAL TO packages/core/markdown/mention-shortcodes.ts.
 * Mobile imports the core copy because packages/ui/ cannot be imported from
 * mobile (Sharing Principles in apps/mobile/CLAUDE.md), and packages/ui/
 * cannot import from packages/core/ (Package Boundary Rules in root
 * CLAUDE.md). If you change the regex / behavior here, change core's copy
 * too — otherwise web and mobile will render legacy mentions differently
 * and the "Counts must agree" parity rule breaks.
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
