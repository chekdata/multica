/** Bind callbacks to operator configuration, never to the inbound request URL. */
export function createCallbackClient(baseURL) {
  const base = new URL(baseURL);
  if (base.protocol !== "https:" || base.username || base.password ||
      base.search || base.hash || baseURL.endsWith("/")) {
    throw new Error("MULTICA_CALLBACK_URL must be the trusted HTTPS callback base without a trailing slash");
  }
  return async function callback(body, method, path, payload) {
    if (body.callback_url !== baseURL) throw new Error("Unexpected callback URL");
    const response = await fetch(`${baseURL}${path}`, {
      method,
      redirect: "error",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${body.callback_token}`,
      },
      body: payload === undefined ? undefined : JSON.stringify(payload),
    });
    const text = await response.text();
    if (response.status === 404) return null;
    if (!response.ok) throw new Error(`Multica answered ${response.status}: ${text}`);
    return text ? JSON.parse(text) : null;
  };
}
