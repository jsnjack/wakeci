module.exports = {
    darkMode: 'class',
    content: [
        "./index.html",
        "./src/**/*.vue",
    ],
    theme: {
        extend: {
            colors: {
                primary: {
                    light: '#A5B4FC',
                    DEFAULT: '#3730A3',
                },
                secondary: {
                    DEFAULT: '#111827',
                    dark: '#01050F',
                },
                gray: {
                    light: '#E5E7EB',
                    border: '#6B7280',
                },
                warning: {
                    DEFAULT: '#FBBF24',
                },
                danger: {
                    DEFAULT: '#B91C1C',
                },
                success: {
                    DEFAULT: '#15803D',
                },
            },
        },
    },
    plugins: [],
};
