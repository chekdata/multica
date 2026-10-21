import assert from "node:assert/strict";
import { afterEach, mock, test } from "node:test";
import { createCallbackClient } from "./callback.mjs";

const baseURL = "https://multica.example/api/v1";
afterEach(() => mock.restoreAll());

test("rejects a request-controlled callback destination before sending credentials", async () => {
  const fetch = mock.method(globalThis, "fetch", async () => { throw new Error("must not fetch"); });
  const callback = createCallbackClient(baseURL);
  await assert.rejects(callback({callback_url: "https://attacker.example", callback_token: "secret"}, "GET", "/issues/1"), /Unexpected callback URL/);
  assert.equal(fetch.mock.callCount(), 0);
});

test("sends only to the configured destination and refuses redirects", async () => {
  const fetch = mock.method(globalThis, "fetch", async () => new Response('{"id":"1"}', {status: 200}));
  const result = await createCallbackClient(baseURL)({callback_url: baseURL, callback_token: "test-token"}, "GET", "/issues/1");
  assert.deepEqual(result, {id: "1"});
  const [url, options] = fetch.mock.calls[0].arguments;
  assert.equal(url, baseURL + "/issues/1");
  assert.equal(options.redirect, "error");
  assert.equal(options.headers.Authorization, "Bearer test-token");
});

test("rejects insecure or ambiguous operator configuration", () => {
  for (const url of ["", "http://localhost/api/v1", "https://user:pass@example.com", baseURL + "?x=1", baseURL + "#fragment", baseURL + "/"]) {
    assert.throws(() => createCallbackClient(url));
  }
});
