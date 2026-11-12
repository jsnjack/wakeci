// chartWidth gives a chart the width of its element, so that it draws in
// pixels and its text keeps its size on any screen. The chart draws once
// width is above 0.
export default {
    data() {
        return { width: 0 };
    },
    mounted() {
        this.width = Math.round(this.$el.getBoundingClientRect().width);
        this.resizeObserver = new ResizeObserver((entries) => {
            const width = Math.round(entries[0].contentRect.width);
            if (width > 0 && width !== this.width) {
                this.width = width;
            }
        });
        this.resizeObserver.observe(this.$el);
    },
    beforeUnmount() {
        this.resizeObserver.disconnect();
    },
};
