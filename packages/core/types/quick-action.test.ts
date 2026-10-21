// @vitest-environment node
import { describe, expect, it } from "vitest";
import { findQuickActionTemplateToken } from "./quick-action";

describe("findQuickActionTemplateToken", () => {
  it.each([
    ["Hello {{name}}", "{{name}}"],
    ["{{outer{{inner}}", "{{outer{{inner}}"],
    ["{{broken} then {{valid}}", "{{valid}}"],
    ["{{}}", "{{}}"],
    ["plain text", null],
    ["{{".repeat(50000), null],
  ])("extracts the first complete token", (input, expected) => {
    expect(findQuickActionTemplateToken(input!)).toBe(expected);
  });
});
