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
                    Search for builds by <b>ID</b>, <b>name</b>, <b>parameters</b>, or <b>status</b>. Keywords are separated by spaces.
                </p>

                <p>
                    Use <code>+</code> to require a keyword, <code>-</code> to exclude it, or quotes for exact phrases. Keywords use OR
                    logic: if any match, the build is returned.
                </p>

                <p>
                    Specific attributes can be targeted using <code>key:value</code> syntax, e.g., <code>status:failed</code> or <code>name:myjob</code>.
                </p>

                <h6>Examples</h6>

                <p>Failed builds</p>
                <code>+status:failed</code>

                <p>Failed or timed out builds</p>
                <code>status:failed "status:timed out"</code>

                <p>Failed builds excluding "test_"</p>
                <code>+status:failed -name:test_</code>

                <p>Builds with parameter "env=prod"</p>
                <code>+env:prod</code>
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
<style scoped ></style>
