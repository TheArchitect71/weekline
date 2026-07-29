import {
  type LeaseClientConfig,
  releaseLease,
  renewLease,
} from "./heartbeat.ts";

Deno.test("manager lease renews and releases only its own instance", async () => {
  const requests: Array<
    {
      method: string;
      path: string;
      authorization: string | null;
      body: unknown;
    }
  > = [];
  const abort = new AbortController();
  const server = Deno.serve({
    hostname: "127.0.0.1",
    port: 0,
    signal: abort.signal,
    onListen() {},
  }, async (request) => {
    requests.push({
      method: request.method,
      path: new URL(request.url).pathname,
      authorization: request.headers.get("authorization"),
      body: request.method === "PUT" ? await request.json() : null,
    });
    return Response.json({
      available: request.method === "PUT",
      activeCount: request.method === "PUT" ? 2 : 1,
    });
  });

  const address = server.addr as Deno.NetAddr;
  const config: LeaseClientConfig = {
    serverUrl: `http://127.0.0.1:${address.port}`,
    hostControlToken: "a".repeat(32),
    instanceId: "550e8400-e29b-41d4-a716-446655440000",
    machineName: "Office-Manager-2",
    appVersion: "0.1.0",
  };

  try {
    const renewed = await renewLease(config);
    const released = await releaseLease(config);
    if (!renewed.available || renewed.activeCount !== 2) {
      throw new Error("renew response was not decoded");
    }
    if (released.available || released.activeCount !== 1) {
      throw new Error("release response was not decoded");
    }
    if (requests.length !== 2) {
      throw new Error(`expected 2 requests, received ${requests.length}`);
    }
    if (requests[0].method !== "PUT" || requests[1].method !== "DELETE") {
      throw new Error("lease methods were incorrect");
    }
    if (requests[0].path !== `/api/host/leases/${config.instanceId}`) {
      throw new Error("lease URL was incorrect");
    }
    if (requests[0].authorization !== `Bearer ${config.hostControlToken}`) {
      throw new Error("control token was missing");
    }
    const body = requests[0].body as {
      machineName?: string;
      appVersion?: string;
    };
    if (
      body.machineName !== config.machineName ||
      body.appVersion !== config.appVersion
    ) throw new Error("host metadata was incorrect");
  } finally {
    abort.abort();
    await server.finished;
  }
});
