export interface LeaseClientConfig {
  serverUrl: string;
  hostControlToken: string;
  instanceId: string;
  machineName: string;
  appVersion: string;
}

export interface HostStatus {
  available: boolean;
  activeCount: number;
}

export async function renewLease(
  config: LeaseClientConfig,
  signal?: AbortSignal,
): Promise<HostStatus> {
  const response = await fetch(leaseUrl(config), {
    method: "PUT",
    signal,
    headers: {
      authorization: `Bearer ${config.hostControlToken}`,
      "content-type": "application/json",
    },
    body: JSON.stringify({
      machineName: config.machineName,
      appVersion: config.appVersion,
    }),
  });
  return decodeStatus(response, "renew");
}

export async function releaseLease(
  config: LeaseClientConfig,
): Promise<HostStatus> {
  const response = await fetch(leaseUrl(config), {
    method: "DELETE",
    headers: { authorization: `Bearer ${config.hostControlToken}` },
  });
  return decodeStatus(response, "release");
}

function leaseUrl(config: LeaseClientConfig): string {
  return `${config.serverUrl}/api/host/leases/${config.instanceId}`;
}

async function decodeStatus(
  response: Response,
  operation: string,
): Promise<HostStatus> {
  if (!response.ok) {
    let detail = `${response.status} ${response.statusText}`;
    try {
      const body = await response.json() as { message?: string };
      if (body.message) detail = body.message;
    } catch {
      // Keep the HTTP status when the controller did not return JSON.
    }
    throw new Error(
      `Unable to ${operation} the Weekline host lease: ${detail}`,
    );
  }
  return await response.json() as HostStatus;
}
