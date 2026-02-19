export interface PermissionStatus {
  cookies: boolean;
  tabs: boolean;
  hostPermissions: boolean;
}

export async function checkPermissions(): Promise<PermissionStatus> {
  const [cookies, tabs, hostPermissions] = await Promise.all([
    chrome.permissions.contains({ permissions: ["cookies"] }),
    chrome.permissions.contains({ permissions: ["tabs"] }),
    chrome.permissions.contains({ origins: ["*://*/*"] }),
  ]);
  return { cookies, tabs, hostPermissions };
}

export async function requestCorePermissions(): Promise<boolean> {
  return chrome.permissions.request({
    permissions: ["cookies", "tabs"],
    origins: ["*://*/*"],
  });
}

export async function hasRequiredPermissions(): Promise<boolean> {
  const status = await checkPermissions();
  return status.cookies && status.tabs && status.hostPermissions;
}
