export const paramsToString = (params, separator = ', ') => {
    return params
        .map((param) => {
            const key = Object.keys(param)[0];
            return `${key.toUpperCase()}=${param[key]}`;
        })
        .join(separator);
};
