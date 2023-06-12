<template>
    <div class="fill center-align no-round">
        {{ getText }}
    </div>
</template>

<script>
const frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
const interval = 130;

export default {
    data: function () {
        return {
            step: 0,
            frameInterval: null,
        };
    },
    computed: {
        getText: function () {
            return frames[this.step];
        },
    },
    mounted() {
        this.frameInterval = setInterval(
            function () {
                const s = this.step + 1;
                if (this.step >= frames.length - 1) {
                    this.step = 0;
                } else {
                    this.step = s;
                }
            }.bind(this),
            interval
        );
    },
    beforeUnmount: function () {
        clearInterval(this.frameInterval);
    },
};
</script>

<style scoped lang="scss"></style>
