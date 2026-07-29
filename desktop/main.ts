import { loadManagerConfig } from "./config.ts";
import {
  type LeaseClientConfig,
  releaseLease,
  renewLease,
} from "./heartbeat.ts";

const config = await loadManagerConfig();
const lease: LeaseClientConfig = {
  ...config,
  instanceId: crypto.randomUUID(),
};

const managerWindow = new Deno.BrowserWindow({
  title: "Weekline Manager",
  width: 1440,
  height: 920,
});

let closing = false;
let renewing = false;
let redirectReady = false;
let statusMessage = "Connecting to the Weekline office host…";
let renewalAbort = new AbortController();

const redirectServer = Deno.serve({ onListen() {} }, () => {
  if (redirectReady) return Response.redirect(`${config.serverUrl}/login`);
  return new Response(statusPage(statusMessage), {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
    },
  });
});

const heartbeat = setInterval(() => void refreshLease(), 15_000);
managerWindow.addEventListener("close", (event) => {
  if (closing) return;
  event.preventDefault();
  closing = true;
  clearInterval(heartbeat);
  renewalAbort.abort();
  void closeManager();
});

await refreshLease();

async function refreshLease(): Promise<void> {
  if (closing || renewing) return;
  renewing = true;
  renewalAbort = new AbortController();
  try {
    const hostStatus = await renewLease(lease, renewalAbort.signal);
    console.log(
      `Weekline employee site enabled by ${config.machineName}; ${hostStatus.activeCount} manager app(s) active.`,
    );
    if (!redirectReady) {
      redirectReady = true;
      managerWindow.reload();
    }
  } catch (error) {
    if (
      closing && error instanceof DOMException && error.name === "AbortError"
    ) return;
    statusMessage = error instanceof Error ? error.message : String(error);
    console.error(statusMessage);
    if (!redirectReady) managerWindow.reload();
  } finally {
    renewing = false;
  }
}

async function closeManager(): Promise<void> {
  try {
    const finalStatus = await releaseLease(lease);
    console.log(
      `Weekline manager closed; ${finalStatus.activeCount} manager app(s) remain.`,
    );
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
  } finally {
    await redirectServer.shutdown();
    managerWindow.close();
  }
}

function statusPage(message: string): string {
  const safeMessage = message
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
  return `<!doctype html>
  <html lang="en">
    <head><meta charset="utf-8"><title>Weekline Manager</title></head>
    <body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#f5f7fb;color:#172033;font:16px system-ui">
      <main style="max-width:520px;padding:32px;text-align:center">
        <h1 style="margin:0 0 12px">Weekline Manager</h1>
        <p>${safeMessage}</p>
        <p style="color:#667085;font-size:13px">The manager app will retry automatically.</p>
      </main>
    </body>
  </html>`;
}
