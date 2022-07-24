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
				    light: '#374151',
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
                    light: '#FEF3C7',
                    DEFAULT: '#FBBF24',
                    dark: '#D97706',
                },
                danger: {
                    light: '#F87171',
                    DEFAULT: '#B91C1C',
                    dark: '#7F1D1D',
                },
                success: {
                    light: '#BBF7D0',
                    DEFAULT: '#15803D',
                    dark: '#166534',
                },
                info: {
                    light: '#A5F3FC',
                    DEFAULT: '#0891B2',
                    dark: '#155E75',
                },
            },
            screens: {
                'max-no-desktop': { max: '768px' },
            },
        },
    },
    plugins: [],
};
