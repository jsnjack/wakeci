import state from "./store/state";

// Guards return where to go instead, or nothing to let the navigation through

export function requireAuth(to) {
    if (!state.auth.isLoggedIn) {
        return {
            path: "/login",
            query: { redirect: to.fullPath },
            replace: true,
        };
    }
}

// requireAdmin keeps the pages only the admin password can use from others
export function requireAdmin() {
    if (!state.auth.admin) {
        return { path: "/settings", replace: true };
    }
}

// requireAdminForAnalytics sends others from the analytics tabs only the
// admin password sees to the analytics of builds
export function requireAdminForAnalytics() {
    if (!state.auth.admin) {
        return { name: "analytics", replace: true };
    }
}
