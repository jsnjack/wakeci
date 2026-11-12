import state from "./store/state";

export function requireAuth(to, from, next) {
    if (!state.auth.isLoggedIn) {
        next({
            path: "/login",
            query: { redirect: to.fullPath },
            replace: true,
        });
    } else {
        next();
    }
}

// requireAdmin keeps the pages only the admin password can use from others
export function requireAdmin(to, from, next) {
    if (!state.auth.admin) {
        next({ path: "/settings", replace: true });
    } else {
        next();
    }
}
