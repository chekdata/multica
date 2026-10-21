/**
 * Strip mention markdown syntax to plain text.
 *
 * Handles:
 * - Simple mentions: `[@Name](mention://agent/id)` → `@Name`
 * - Escaped brackets in names: `[@David\[TF\]](mention://agent/id)` → `@David[TF]`
 * - Issue mentions (no @): `[MUL-123](mention://issue/id)` → `MUL-123`
 * - Does NOT touch regular markdown links: `[docs](https://...)` stays unchanged
 * - Does NOT touch backslash-escaped mentions: `\[@Name](mention://...)` stays unchanged
 *
 * Scan once so malformed labels cannot trigger regex backtracking.
 */
export function stripMentionMarkdown(text: string): string {
  const parts: string[] = [];
  const mentionPrefix = "(mention://";
  let literalStart = 0;
  let labelStart = -1;
  for (let i = 0; i < text.length; i++) {
    if (labelStart >= 0 && text[i] === "\\") {
      i++;
      continue;
    }
    if (text[i] === "[" && labelStart < 0 && text[i - 1] !== "\\") {
      labelStart = i;
      continue;
    }
    if (text[i] !== "]" || labelStart < 0) continue;

    if (i > labelStart + 1 && text.startsWith(mentionPrefix, i + 1)) {
      const typeStart = i + 1 + mentionPrefix.length;
      let typeEnd = typeStart;
      while (typeEnd < text.length && /\w/.test(text.charAt(typeEnd))) typeEnd++;
      if (typeEnd > typeStart && text[typeEnd] === "/") {
        const end = text.indexOf(")", typeEnd + 1);
        if (end < 0) break;
        if (end > typeEnd + 1) {
          parts.push(
            text.slice(literalStart, labelStart),
            text.slice(labelStart + 1, i).replace(/\\\[/g, "[").replace(/\\\]/g, "]"),
          );
          literalStart = end + 1;
          i = end;
        }
      }
    }
    labelStart = -1;
  }
  parts.push(text.slice(literalStart));
  return parts.join("");
}
