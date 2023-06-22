<template>
    <div id="helpView">
        <div class="tabs min right-align">
            <a
                class="active"
                @click.prevent="toggleTab"
                >Job syntax</a
            >
            <a @click.prevent="toggleTab">Search syntax</a>
            <a @click.prevent="toggleTab">REST API</a>
        </div>
        <div class="page padding active">
            <SyntaxViewer />
        </div>
        <div class="page padding">
            <article>
                <p>
                    The search function allows you to retrieve specific builds based on their attributes, providing you with a flexible and precise filtering
                    mechanism. With this feature, you can search for builds using the following criteria:
                </p>

                <p class="italic">ID, name, parameters, or status</p>

                <p>
                    The search functionality supports the use of space-separated keywords, where any of these keywords can be present in the mentioned
                    attributes to qualify a build for retrieval. Additionally, you can include specific keywords by prefixing them with a "+" sign, ensuring
                    that only builds containing those keywords are returned. Conversely, you can exclude certain keywords from the results by prefixing them
                    with a "-" sign. To make your searches even more convenient, phrases can be enclosed in single or double quotes. This allows you to search
                    for exact phrases within the build attributes.
                </p>

                <p>Here are some examples to illustrate how this search feature works:</p>

                <h6>Example 1: Retrieving all failed builds</h6>
                <code>+failed</code>

                <h6>Example 2: Retrieving all failed and timed out builds</h6>
                <code>failed "timed out"</code>

                <h6>Example 2: Retrieving all failed builds which don't contain "test_"</h6>
                <code>+failed -test_</code>
            </article>
        </div>
        <div class="page padding">
            <div class="fill medium-height middle-align center-align">
                <div class="center-align">
                    <i class="extra">assistant_direction</i>
                    <h5>
                        <a
                            class="link"
                            href="/docs/api/"
                            target="_blank"
                            >Open REST API documentation in a new window</a
                        >
                    </h5>
                </div>
            </div>
        </div>
    </div>
</template>

<script>
import SyntaxViewer from "@/components/SyntaxViewer.vue";

export default {
    components: {
        SyntaxViewer,
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Help");
    },
    methods: {
        toggleTab(event) {
            // Deactivate all tabs
            let pos = 0;
            document.querySelectorAll("#helpView .tabs a").forEach((a, id) => {
                if (a === event.target) {
                    pos = id;
                }
                a.classList.remove("active");
            });
            // Activate the clicked tab
            event.target.classList.add("active");
            // Activate the clicked tab content
            document.querySelectorAll("#helpView .page").forEach((p, id) => {
                p.classList.remove("active");
                if (id === pos) {
                    p.classList.add("active");
                }
            });
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss"></style>
