module.exports = {
    darkMode: 'class',
    content: ['./index.html', './src/**/*.vue'],
    theme: {
        extend: {
            colors: {
                primary: {
                    light: '#A5B4FC',
                    DEFAULT: '#3730A3',
                    dark: '#312E81',
                },
                secondary: {
                    DEFAULT: '#111827',
                    dark: '#01050F',
                },
                gray: {
                    'extra-light': '#F9FAFB',
                    light: '#E5E7EB',
                    border: '#6B7280',
                    'border-dark': '#4B5563',
                },
                warning: {
                    DEFAULT: '#FBBF24',
                },
                danger: {
                    DEFAULT: '#B91C1C',
                    dark: '#7F1D1D',
                },
                success: {
                    DEFAULT: '#15803D',
                },
                info: {
                    DEFAULT: '#0891B2',
                },
            },
        },
    },
    plugins: [],
};
