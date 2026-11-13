# Settings: Wakefile.yaml and the settings page

Read this before adding a setting, or changing the settings file or
`--import-settings`.

## Who owns what

Each setting has one owner; nothing is mirrored between the two.

- **Wakefile.yaml** holds what the server needs to start and what operations
  owns: port, hostname, directories, the secrets file, the time zone, the
  identity provider and `draintimeout`, which systemd's `TimeoutStopSec`
  must match. It is read at startup; a change needs a restart.
- **The settings page** (the database) holds what admins change while
  wakeci runs, recorded in the audit log: the password, concurrent builds,
  builds kept, the API key limits, the single sign-on groups and the rule
  for people in no group, and the API keys.

A new setting goes to the file if wakeci can't start or bind without it,
or if it must match something outside wakeci; otherwise to the page.

The single sign-on groups are on the page only, also those of a server
that used to set them in `sso.groups` (wakeci 2.41 and older). wakeci
ignores that key, so a Wakefile.yaml kept for a rollback can still have
it. The settings file below is the way to keep groups in git.

## The settings page

The page starts with the version and the update button, then has a
section each for builds, API key limits, the admin password, single
sign-on groups and the settings file, divided by lines rather than cards,
like the other forms. The build limits, API
key limits and password share one Save button below them.
Groups and file imports have their own actions. The password, groups and
settings file are shown only to the admin password.

API clients can read runtime configuration in the `server` field of
`/api/settings` (`ServerConfigData`): the configuration file's path, port,
hostname, directories, the secrets file with its count (never the secret
values), time zone, drain timeout and SSO metadata and signing
certificates. The page shows only the settings it can edit and the update
status; it has no read-only Server panel or certificate expiry warnings.

## The settings file

`GET /api/settings/export` writes the settings of the page as YAML
(`settings_file.go`): `concurrentBuilds`, `buildHistorySize`,
`apiKeyRate`, `apiKeyBurst`, `ssoGroups` and `ssoNoGroup` (`null` while it
is off). It never holds the password, API keys or sessions. Only the admin
password exports and imports, since the file has the groups.

Importing sets what the file has and leaves the rest, so a file with only
`ssoGroups` changes only the groups: `ssoGroups` replaces every group,
`ssoNoGroup: null` turns the rule off, and a missing key changes nothing.
`ssoGroups: []` removes every group, but an empty `ssoGroups:` (null) is
refused, since a forgotten list would lock out everyone who signs in with
single sign-on.
The whole file is checked first, with the rules of the page (a grant goes
through `grantFromForm`), unknown keys refused, and saved in one
transaction, so a file with a mistake changes nothing; errors name the
setting, never a value.

There are two ways in:

- **The page** (`POST /api/settings/import`, audited as `settings.import`),
  which applies it at once like the page's own changes.
- **`wakeci --import-settings file.yaml`** for a stopped server, also one
  that never ran: it opens the database (refusing after 2 seconds if a
  running wakeci holds it), creates what a first start would
  (`bootstrapDB`), imports, writes an audit line and exits. `bootstrapDB`
  writes each default only if it is missing, so the first start keeps the
  imported values.
