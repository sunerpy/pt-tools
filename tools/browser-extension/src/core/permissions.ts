export interface PermissionStatus {
  cookies: boolean;
  tabs: boolean;
  hostPermissions: boolean;
  webNavigation: boolean;
}

export async function checkPermissions(): Promise<PermissionStatus> {
  const [cookies, tabs, hostPermissions, webNavigation] = await Promise.all([
    chrome.permissions.contains({ permissions: ["cookies"] }),
    chrome.permissions.contains({ permissions: ["tabs"] }),
    chrome.permissions.contains({ origins: ["*://*/*"] }),
    chrome.permissions.contains({ permissions: ["webNavigation"] }),
  ]);
  return { cookies, tabs, hostPermissions, webNavigation };
}

export async function requestCorePermissions(): Promise<boolean> {
  return chrome.permissions.request({
    permissions: ["cookies", "tabs", "webNavigation"],
    origins: ["*://*/*"],
  });
}

export async function hasRequiredPermissions(): Promise<boolean> {
  const status = await checkPermissions();
  return status.cookies && status.tabs && status.hostPermissions;
}

export async function hasWebNavigationPermission(): Promise<boolean> {
  return chrome.permissions.contains({ permissions: ["webNavigation"] });
}
