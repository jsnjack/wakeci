import state from "./store/state";
import { grantHasScope } from "./store/utils";

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

// requireUsageAccess sends those without write from the analytics of
// requests, which only write and the admin password see, to the analytics of
// builds
export function requireUsageAccess() {
    if (!state.auth.admin && !state.auth.grants.some((grant) => grantHasScope(grant, "write"))) {
        return { name: "analytics", replace: true };
    }
}
