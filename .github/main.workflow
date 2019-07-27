workflow "New workflow" {
  on = "push"
  resolves = ["HTTP client"]
}

action "HTTP client" {
  uses = "swinton/httpie.action@69125d73caa2c6821f6a41a86112777a37adc171"
  args = ["--auth=:$WAKECI_PASS", "POST", "https://ci.yauhen.space/api/job/release_wakeci/run?COMMIT_ID=$GITHUB_SHA"]
  secrets = ["WAKECI_PASS"]
}
