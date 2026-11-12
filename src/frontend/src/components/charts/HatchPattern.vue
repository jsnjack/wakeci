<template>
    <defs>
        <pattern
            :id="id"
            width="4.5"
            height="4.5"
            patternUnits="userSpaceOnUse"
            patternTransform="rotate(45)"
        >
            <rect
                width="4.5"
                height="4.5"
                class="hatch-fill"
            />
            <rect
                width="1.5"
                height="4.5"
                class="hatch-line"
            />
        </pattern>
    </defs>
</template>

<script>
// HatchPattern is the diagonal hatch of failed builds, so that they differ
// from finished ones by more than colour. A chart fills its failed marks
// with url(#id)
export default {
    props: {
        id: {
            type: String,
            required: true,
        },
    },
};
</script>
