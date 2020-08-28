import state from "./store/state";


export function requireAuth(to, from, next) {
    if (!state.auth.isLoggedIn) {
        next({
            path: "/login",
            query: {redirect: to.fullPath},
            replace: true,
        });
    } else {
        next();
    }
}
