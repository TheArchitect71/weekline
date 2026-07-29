export interface ManagerConfig {
  serverUrl: string;
  hostControlToken: string;
  appVersion: string;
  machineName: string;
}

interface StoredManagerConfig {
  serverUrl?: string;
  hostControlToken?: string;
  appVersion?: string;
  machineName?: string;
}

export async function loadManagerConfig(): Promise<ManagerConfig> {
  const stored = await readStoredConfig();
  const serverUrl =
    (Deno.env.get("WEEKLINE_SERVER_URL") ?? stored.serverUrl ?? "").replace(
      /\/$/,
      "",
    );
  const hostControlToken = Deno.env.get("WEEKLINE_HOST_CONTROL_TOKEN") ??
    stored.hostControlToken ?? "";
  const appVersion = Deno.env.get("WEEKLINE_APP_VERSION") ??
    stored.appVersion ?? "0.1.0-dev";
  const machineName = Deno.env.get("WEEKLINE_MACHINE_NAME") ??
    stored.machineName ?? Deno.hostname();

  if (
    !serverUrl.startsWith("https://") &&
    !serverUrl.startsWith("http://127.0.0.1") &&
    !serverUrl.startsWith("http://localhost")
  ) {
    throw new Error(
      "Configure WEEKLINE_SERVER_URL with the office HTTPS URL (or localhost for development).",
    );
  }
  if (hostControlToken.length < 32) {
    throw new Error(
      "Configure WEEKLINE_HOST_CONTROL_TOKEN with the same 32+ character secret used by the office host.",
    );
  }
  if (!machineName.trim()) {
    throw new Error("The manager desktop needs a machine name.");
  }
  return {
    serverUrl,
    hostControlToken,
    appVersion,
    machineName: machineName.trim(),
  };
}

async function readStoredConfig(): Promise<StoredManagerConfig> {
  const path = Deno.env.get("WEEKLINE_MANAGER_CONFIG") ?? defaultConfigPath();
  try {
    return JSON.parse(await Deno.readTextFile(path)) as StoredManagerConfig;
  } catch (error) {
    if (error instanceof Deno.errors.NotFound) return {};
    if (error instanceof SyntaxError) {
      throw new Error(`Manager configuration is not valid JSON: ${path}`);
    }
    throw error;
  }
}

function defaultConfigPath(): string {
  if (Deno.build.os === "windows") {
    const appData = Deno.env.get("APPDATA") ?? ".";
    return `${appData}\\Weekline\\manager.json`;
  }
  const home = Deno.env.get("HOME") ?? ".";
  if (Deno.build.os === "darwin") {
    return `${home}/Library/Application Support/Weekline/manager.json`;
  }
  return `${
    Deno.env.get("XDG_CONFIG_HOME") ?? `${home}/.config`
  }/weekline/manager.json`;
}
