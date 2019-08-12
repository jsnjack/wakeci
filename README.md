wakeci
======

# Basic functionality
- [x] Abort builds
- [x] Builds support parameters
- [x] Auth
- [x] Artifacts

# Impovements high
- [ ] ~~Parallel tasks(?)~~
- [x] ~~Post tasks (always executed)~~ Replaced with on status change handlers
- [x] Reload logs messes up order
- [x] Add cron-like features
- [x] Add default env variables: workspace, build_id ...
- [x] Handlers on failure, on abort..?
- [ ] A token to only run task ang get status update
- [x] Cleanups

# Improvements low
- [x] Use monospace font for logs
- [x] Add gzip middleware
- [x] Create settings page
- [x] Create/edit job with editor
- [x] Default template for new jobs
- [x] When job was started
- [ ] Timestamps to logs
- [x] Total time to finish build/task
- [ ] ~~Collapse logs on the Build page~~

# Improvements super low
- [ ] Make it all pretty
- [ ] Do not subscribe on the builds page if build is finished
- [ ] Think about StatusUpdate for tasks, looks a bit overdesigned now
- [ ] PWA?

# Packaging
- [x] Add support for Lets encrypt SSL cert
- [x] Package all in one binary
