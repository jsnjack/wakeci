import { grantAllows, grantHasScope } from "./utils";

const getters = {
    // can reports whether the user may use scope on the job named job
    can: (state) => (scope, job) => state.auth.admin || state.auth.grants.some((grant) => grantAllows(grant, scope, job)),
    // canAny reports whether the user has scope on some job
    canAny: (state) => (scope) => state.auth.admin || state.auth.grants.some((grant) => grantHasScope(grant, scope)),
};

export default getters;
