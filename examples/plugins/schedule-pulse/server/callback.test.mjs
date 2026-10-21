import assert from "node:assert/strict";
import { afterEach, mock, test } from "node:test";
import { createCallbackClient } from "./callback.mjs";

const baseURL = "https://multica.example/api/v1";
const request = { callback_url: baseURL, callback_token: "test-token" };
afterEach(() => mock.restoreAll());

test("rejects a request-controlled destination before sending credentials", async () => {
  const fetch = mock.method(globalThis, "fetch", async () => { throw new Error("must not fetch"); });
  const callback = createCallbackClient(baseURL);
  await assert.rejects(callback({ ...request, callback_url: "https://attacker.example" }, "GET", "/storage/workspace/last_pulse"), /Unexpected callback URL/);
  assert.equal(fetch.mock.callCount(), 0);
});

test("binds the destination, refuses redirects, and preserves the storage response", async () => {
  const fetch = mock.method(globalThis, "fetch", async () => new Response('{"value":"pulse"}', { status: 200 }));
  const result = await createCallbackClient(baseURL)(request, "GET", "/storage/workspace/last_pulse");
  assert.deepEqual(result, { value: "pulse" });
  const [url, options] = fetch.mock.calls[0].arguments;
  assert.equal(url, baseURL + "/storage/workspace/last_pulse");
  assert.equal(options.redirect, "error");
  assert.equal(options.headers.Authorization, "Bearer test-token");
});

test("preserves missing storage and empty success semantics", async () => {
  const callback = createCallbackClient(baseURL);
  for (const status of [404, 204]) {
    mock.method(globalThis, "fetch", async () => new Response(null, { status }));
    assert.equal(await callback(request, "GET", "/storage/workspace/last_pulse"), null);
    mock.restoreAll();
  }
});

test("rejects insecure or ambiguous operator configuration", () => {
  for (const url of ["", "http://localhost/api/v1", "https://user:pass@example.com", baseURL + "?x=1", baseURL + "#fragment", baseURL + "/"]) {
    assert.throws(() => createCallbackClient(url));
  }
});
