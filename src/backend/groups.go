package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// SSOGroup lets the members of a single sign-on group log in and do what
// its grant allows
type SSOGroup struct {
	// Name is the group as the identity provider names it in the groups
	// attribute. Logins match it ignoring case.
	Name string `json:"name"`
	Grant
	UpdatedAt time.Time `json:"updatedAt"`
}

// SSOGroupsData lists the single sign-on groups for the admin password
type SSOGroupsData struct {
	Groups []SSOGroupInfo `json:"groups"`
	// NoGroup is what people in none of the groups may do, null if they
	// can't sign in
	NoGroup *SSOGroupInfo `json:"noGroup" extensions:"x-nullable"`
	// SSOEnabled is false until sso.idpmetadata is set; groups wait for it
	SSOEnabled bool `json:"ssoEnabled"`
}

// SSOGroupInfo is a group with how many of its members are signed in
type SSOGroupInfo struct {
	SSOGroup
	SignedIn int `json:"signedIn"`
}

const maxSSOGroupNameLength = 200

var (
	errSSOGroupExists   = errors.New("a group with this name exists already")
	errSSOGroupNotFound = errors.New("no such group")
)

// ssoNoGroupKey in GlobalBucket holds the rule for people in none of the
// groups, as an SSOGroup without a name; without it they can't sign in. It
// isn't in the groups bucket, so no group's name can stand for it.
var ssoNoGroupKey = []byte("ssoNoGroup")

// ssoGroupsSeededKey in GlobalBucket records that the groups of sso.groups
// were added, so they aren't added again after the admin deleted them
var ssoGroupsSeededKey = []byte("ssoGroupsSeeded")

// onSSOAccessChange is called after a group changed, so that live
// connections opened with the old access are closed
var onSSOAccessChange func()

func ssoGroupKey(name string) []byte {
	return []byte(strings.ToLower(strings.TrimSpace(name)))
}

// seedSSOGroups adds the groups of sso.groups with access to everything,
// once: afterwards the groups are managed in the settings
func seedSSOGroups(names []string, now time.Time) error {
	if len(names) == 0 {
		return nil
	}
	seeded := false
	err := DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		if gb.Get(ssoGroupsSeededKey) != nil {
			return nil
		}
		b := tx.Bucket(SSOGroupsBucket)
		for _, name := range names {
			if b.Get(ssoGroupKey(name)) != nil {
				continue
			}
			group := SSOGroup{
				Name:      strings.TrimSpace(name),
				Grant:     Grant{Scopes: slices.Clone(apiKeyScopes), AllJobs: true, Jobs: []string{}, Patterns: []string{}},
				UpdatedAt: now,
			}
			if err := putSSOGroup(b, &group); err != nil {
				return err
			}
		}
		seeded = true
		return gb.Put(ssoGroupsSeededKey, []byte("true"))
	})
	if err != nil {
		return err
	}
	if seeded {
		L.Info("added the groups of sso.groups with access to all jobs", "groups", names)
		return nil
	}
	// The groups are in the database now; editing the config file has no
	// effect, which would be surprising without this
	L.Warn("sso.groups is only read once, on the first start; manage the groups in the settings")
	return nil
}

func putSSOGroup(b *bolt.Bucket, group *SSOGroup) error {
	data, err := json.Marshal(group)
	if err != nil {
		return fmt.Errorf("encode group %q: %w", group.Name, err)
	}
	return b.Put(ssoGroupKey(group.Name), data)
}

// listSSOGroups returns the groups sorted by name
func listSSOGroups() ([]SSOGroup, error) {
	groups := []SSOGroup{}
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(SSOGroupsBucket).ForEach(func(_, v []byte) error {
			var group SSOGroup
			if err := json.Unmarshal(v, &group); err != nil {
				return fmt.Errorf("decode group: %w", err)
			}
			groups = append(groups, group)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(groups, func(a, b SSOGroup) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return groups, nil
}

// ssoGroupsAccess is what a member of groups may do now: the union of the
// grants of those groups that exist, or, if none does, the rule for people
// in no group
func ssoGroupsAccess(groups []string) (*Access, error) {
	access := &Access{Grants: []Grant{}, subject: "Your account", groups: slices.Clone(groups)}
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(SSOGroupsBucket)
		for _, name := range groups {
			data := b.Get(ssoGroupKey(name))
			if data == nil {
				continue
			}
			var group SSOGroup
			if err := json.Unmarshal(data, &group); err != nil {
				return fmt.Errorf("decode group %q: %w", name, err)
			}
			access.Grants = append(access.Grants, group.Grant)
		}
		if len(access.Grants) > 0 {
			return nil
		}
		rule, err := ssoNoGroupRule(tx)
		if err != nil || rule == nil {
			return err
		}
		access.Grants = append(access.Grants, rule.Grant)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return access, nil
}

// ssoNoGroupRule returns the rule for people in none of the groups, nil if
// they can't sign in
func ssoNoGroupRule(tx *bolt.Tx) (*SSOGroup, error) {
	data := tx.Bucket(GlobalBucket).Get(ssoNoGroupKey)
	if data == nil {
		return nil, nil
	}
	var rule SSOGroup
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("decode the rule for people in no group: %w", err)
	}
	return &rule, nil
}

// ssoAdmits reports whether someone in groups may sign in: one of the
// groups exists, or the rule lets people in no group in
func ssoAdmits(groups []string) bool {
	admits := false
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(SSOGroupsBucket)
		admits = tx.Bucket(GlobalBucket).Get(ssoNoGroupKey) != nil ||
			slices.ContainsFunc(groups, func(name string) bool { return b.Get(ssoGroupKey(name)) != nil })
		return nil
	})
	if err != nil {
		L.Error("find sso groups", "err", err)
		return false
	}
	return admits
}

// ssoGroupName checks the name of a new group
func ssoGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", fmt.Errorf("enter the group's name")
	case utf8.RuneCountInString(name) > maxSSOGroupNameLength:
		return "", fmt.Errorf("the name is longer than %d characters", maxSSOGroupNameLength)
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", fmt.Errorf("the name can't contain control characters")
	}
	return name, nil
}

// ssoGroupNameParam reads the group name from the URL
func ssoGroupNameParam(r *http.Request) (string, error) {
	name := chi.URLParam(r, "name")
	if r.URL.RawPath != "" {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			return "", fmt.Errorf("decode group name: %w", err)
		}
		name = decoded
	}
	return name, nil
}

func ssoGroupAuditAttrs(group *SSOGroup) []any {
	return append([]any{"group", group.Name}, ssoGrantAuditAttrs(group.Grant)...)
}

func ssoGrantAuditAttrs(grant Grant) []any {
	attrs := []any{"scopes", strings.Join(grant.Scopes, ","), "allJobs", grant.AllJobs}
	if !grant.AllJobs {
		attrs = append(attrs, auditListAttrs("jobs", grant.Jobs)...)
		attrs = append(attrs, auditListAttrs("patterns", grant.Patterns)...)
	}
	return attrs
}

func ssoAccessChanged() {
	if onSSOAccessChange != nil {
		onSSOAccessChange()
	}
}

// requireAdmin lets only the admin password through
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := requestLogger(r)
		if access := accessFromContext(r.Context()); access == nil || !access.Full {
			logger.Warn("admin route used without the admin password")
			writePlainText(w, logger, http.StatusForbidden, "Only the admin password can do this")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleSSOGroupsList lists the single sign-on groups and what they may do
// @Summary      List the single sign-on groups and their permissions
// @Description  Only with the admin password. signedIn counts the people with a current session in the group. Not available with an API key
// @Tags         groups
// @Produce      json
// @Success      200      {object}   SSOGroupsData
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/groups/ [get]
func HandleSSOGroupsList(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	groups, err := listSSOGroups()
	if err != nil {
		logger.Error("list sso groups", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	var rule *SSOGroup
	if err := DB.View(func(tx *bolt.Tx) error {
		rule, err = ssoNoGroupRule(tx)
		return err
	}); err != nil {
		logger.Error("read the rule for people in no group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	members := GlobalSessionStorage.GroupMembers()
	data := SSOGroupsData{Groups: []SSOGroupInfo{}, SSOEnabled: Config.SSO.Enabled()}
	known := map[string]bool{}
	for _, group := range groups {
		data.Groups = append(data.Groups, SSOGroupInfo{SSOGroup: group, SignedIn: members[strings.ToLower(group.Name)]})
		known[strings.ToLower(group.Name)] = true
	}
	if rule != nil {
		inNoGroup := GlobalSessionStorage.CountSSOPeople(func(groups []string) bool {
			return !slices.ContainsFunc(groups, func(name string) bool { return known[strings.ToLower(name)] })
		})
		data.NoGroup = &SSOGroupInfo{SSOGroup: *rule, SignedIn: inNoGroup}
	}
	writeJSON(w, logger, data)
}

// HandleSSOGroupCreate lets a single sign-on group log in
// @Summary      Let a single sign-on group log in, with permissions
// @Description  Only with the admin password. Not available with an API key
// @Tags         groups
// @Accept       x-www-form-urlencoded
// @Produce      json
// @Param        name      formData   string   true    "Group name as the identity provider sends it; matched ignoring case"
// @Param        scope     formData   []string true    "read, run and/or write" collectionFormat(multi)
// @Param        allJobs   formData   boolean  false   "Cover all jobs, including jobs added later; required for write"
// @Param        job       formData   []string false   "Job the group may use" collectionFormat(multi)
// @Param        pattern   formData   []string false   "Pattern of job names, * matches anything" collectionFormat(multi)
// @Success      200       {object}   SSOGroup
// @Failure      400       {string}   string
// @Failure      401      {string}   string
// @Failure      403       {string}   string
// @Failure      409       {string}   string
// @Failure      429      {string}   string
// @Failure      500       {string}   string
// @Router       /api/groups/ [post]
func HandleSSOGroupCreate(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if err := r.ParseForm(); err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	name, err := ssoGroupName(r.Form.Get("name"))
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	grant, err := grantFromForm(r.Form)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	group := &SSOGroup{Name: name, Grant: grant, UpdatedAt: time.Now()}
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SSOGroupsBucket)
		if b.Get(ssoGroupKey(name)) != nil {
			return errSSOGroupExists
		}
		return putSSOGroup(b, group)
	})
	if errors.Is(err, errSSOGroupExists) {
		writePlainText(w, logger, http.StatusConflict, "A group named "+name+" exists already")
		return
	}
	if err != nil {
		logger.Error("save sso group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("sso group added", "group", name, "scopes", grant.Scopes)
	addAuditAttrs(r, ssoGroupAuditAttrs(group)...)
	ssoAccessChanged()
	writeJSON(w, logger, group)
}

// HandleSSOGroupUpdate changes what a single sign-on group may do
// @Summary      Change the permissions of a single sign-on group
// @Description  Only with the admin password. Takes effect on the members' next request. Not available with an API key
// @Tags         groups
// @Accept       x-www-form-urlencoded
// @Produce      json
// @Param        name      path       string   true    "Group name"
// @Param        scope     formData   []string true    "read, run and/or write" collectionFormat(multi)
// @Param        allJobs   formData   boolean  false   "Cover all jobs, including jobs added later; required for write"
// @Param        job       formData   []string false   "Job the group may use" collectionFormat(multi)
// @Param        pattern   formData   []string false   "Pattern of job names, * matches anything" collectionFormat(multi)
// @Success      200       {object}   SSOGroup
// @Failure      400       {string}   string
// @Failure      401      {string}   string
// @Failure      403       {string}   string
// @Failure      404       {string}   string
// @Failure      429      {string}   string
// @Failure      500       {string}   string
// @Router       /api/groups/{name} [post]
func HandleSSOGroupUpdate(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	name, err := ssoGroupNameParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	if err := r.ParseForm(); err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	grant, err := grantFromForm(r.Form)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	var group SSOGroup
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SSOGroupsBucket)
		data := b.Get(ssoGroupKey(name))
		if data == nil {
			return errSSOGroupNotFound
		}
		if err := json.Unmarshal(data, &group); err != nil {
			return fmt.Errorf("decode group %q: %w", name, err)
		}
		group.Grant = grant
		group.UpdatedAt = time.Now()
		return putSSOGroup(b, &group)
	})
	if errors.Is(err, errSSOGroupNotFound) {
		writePlainText(w, logger, http.StatusNotFound, "No group named "+name)
		return
	}
	if err != nil {
		logger.Error("save sso group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("sso group changed", "group", group.Name, "scopes", grant.Scopes)
	addAuditAttrs(r, ssoGroupAuditAttrs(&group)...)
	ssoAccessChanged()
	writeJSON(w, logger, &group)
}

// HandleSSOGroupDelete stops a single sign-on group from logging in
// @Summary      Remove a single sign-on group
// @Description  Only with the admin password. Its members lose its permissions on their next request. Not available with an API key
// @Tags         groups
// @Produce      plain
// @Param        name      path       string   true    "Group name"
// @Success      200       {string}   string
// @Failure      401      {string}   string
// @Failure      403       {string}   string
// @Failure      404       {string}   string
// @Failure      429      {string}   string
// @Failure      500       {string}   string
// @Router       /api/groups/{name} [delete]
func HandleSSOGroupDelete(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	name, err := ssoGroupNameParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SSOGroupsBucket)
		if b.Get(ssoGroupKey(name)) == nil {
			return errSSOGroupNotFound
		}
		return b.Delete(ssoGroupKey(name))
	})
	if errors.Is(err, errSSOGroupNotFound) {
		writePlainText(w, logger, http.StatusNotFound, "No group named "+name)
		return
	}
	if err != nil {
		logger.Error("delete sso group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("sso group removed", "group", name)
	ssoAccessChanged()
}

// HandleSSONoGroupSet lets people in none of the groups sign in, with
// permissions
// @Summary      Let people in none of the single sign-on groups sign in, with permissions
// @Description  Only with the admin password. Someone whose groups match none in the settings gets these permissions; a member of a group gets only the groups' permissions. Sets the rule or changes it, on the next request of those it covers. Not available with an API key
// @Tags         groups
// @Accept       x-www-form-urlencoded
// @Produce      json
// @Param        scope     formData   []string true    "read, run and/or write" collectionFormat(multi)
// @Param        allJobs   formData   boolean  false   "Cover all jobs, including jobs added later; required for write"
// @Param        job       formData   []string false   "Job they may use" collectionFormat(multi)
// @Param        pattern   formData   []string false   "Pattern of job names, * matches anything" collectionFormat(multi)
// @Success      200       {object}   SSOGroup
// @Failure      400       {string}   string
// @Failure      401       {string}   string
// @Failure      403       {string}   string
// @Failure      429       {string}   string
// @Failure      500       {string}   string
// @Router       /api/nogroup [post]
func HandleSSONoGroupSet(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if err := r.ParseForm(); err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	grant, err := grantFromForm(r.Form)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	rule := &SSOGroup{Grant: grant, UpdatedAt: time.Now()}
	err = DB.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rule)
		if err != nil {
			return fmt.Errorf("encode the rule for people in no group: %w", err)
		}
		return tx.Bucket(GlobalBucket).Put(ssoNoGroupKey, data)
	})
	if err != nil {
		logger.Error("save the rule for people in no group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("people in no group may sign in", "scopes", grant.Scopes)
	addAuditAttrs(r, ssoGrantAuditAttrs(grant)...)
	ssoAccessChanged()
	writeJSON(w, logger, rule)
}

// HandleSSONoGroupDelete stops people in none of the groups from signing in
// @Summary      Stop people in none of the single sign-on groups from signing in
// @Description  Only with the admin password. Those it covered lose its permissions on their next request; nothing happens if they couldn't sign in. Not available with an API key
// @Tags         groups
// @Produce      plain
// @Success      200       {string}   string
// @Failure      401       {string}   string
// @Failure      403       {string}   string
// @Failure      429       {string}   string
// @Failure      500       {string}   string
// @Router       /api/nogroup [delete]
func HandleSSONoGroupDelete(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Delete(ssoNoGroupKey)
	})
	if err != nil {
		logger.Error("delete the rule for people in no group", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("people in no group can't sign in")
	ssoAccessChanged()
}

// groupOfURL names the group a request acts on in the audit log
func groupOfURL(_ *http.Request, param func(string) string) []any {
	name, err := url.PathUnescape(param("name"))
	if err != nil {
		name = param("name")
	}
	return []any{"group", name}
}
