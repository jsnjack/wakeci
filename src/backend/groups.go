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

// SSOGroupsData lists the single sign-on groups
type SSOGroupsData struct {
	Groups []SSOGroup `json:"groups"`
	// CanEdit is true for the admin password, which alone may change them
	CanEdit bool `json:"canEdit"`
}

const maxSSOGroupNameLength = 200

var (
	errSSOGroupExists   = errors.New("a group with this name exists already")
	errSSOGroupNotFound = errors.New("no such group")
)

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
// grants of those groups that exist
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	return access, nil
}

// knownSSOGroup reports whether name is one of the groups that may log in
func knownSSOGroup(name string) bool {
	known := false
	err := DB.View(func(tx *bolt.Tx) error {
		known = tx.Bucket(SSOGroupsBucket).Get(ssoGroupKey(name)) != nil
		return nil
	})
	if err != nil {
		L.Error("find sso group", "err", err)
		return false
	}
	return known
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
	attrs := []any{"group", group.Name, "scopes", strings.Join(group.Scopes, ","), "allJobs", group.AllJobs}
	if !group.AllJobs {
		attrs = append(attrs, auditListAttrs("jobs", group.Jobs)...)
		attrs = append(attrs, auditListAttrs("patterns", group.Patterns)...)
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
// @Description  With the admin password, or for a user with the write scope (read-only)
// @Tags         groups
// @Produce      json
// @Success      200      {object}   SSOGroupsData
// @Failure      403      {string}   string
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
	access := accessFromContext(r.Context())
	writeJSON(w, logger, SSOGroupsData{Groups: groups, CanEdit: access != nil && access.Full})
}

// HandleSSOGroupCreate lets a single sign-on group log in
// @Summary      Let a single sign-on group log in, with permissions
// @Description  Only with the admin password
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
// @Failure      403       {string}   string
// @Failure      409       {string}   string
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
// @Description  Only with the admin password. Takes effect on the members' next request.
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
// @Failure      403       {string}   string
// @Failure      404       {string}   string
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
// @Description  Only with the admin password. Its members lose its permissions on their next request.
// @Tags         groups
// @Produce      plain
// @Param        name      path       string   true    "Group name"
// @Success      200       {string}   string
// @Failure      403       {string}   string
// @Failure      404       {string}   string
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

// groupOfURL names the group a request acts on in the audit log
func groupOfURL(_ *http.Request, param func(string) string) []any {
	name, err := url.PathUnescape(param("name"))
	if err != nil {
		name = param("name")
	}
	return []any{"group", name}
}
