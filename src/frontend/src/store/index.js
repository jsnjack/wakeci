import {createStore} from "vuex";
import state from "./state";
import actions from "./actions";
import getters from "./getters";
import mutations from "./mutations";

const debug = import.meta.env.NODE_ENV !== "production";

export const store = createStore({
    state: state,
    getters: getters,
    actions: actions,
    mutations: mutations,
    strict: debug,
});
