package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Builds started by other builds (F2 in docs/ui-roadmap.md). A job that
// starts others adds WAKE_PARENT=$WAKE_BUILD_ID to the run call, and the new
// build records its parent and the root, the first build of the chain. Not
// "parent", which a job may have as a parameter.
const (
	parentParam = "WAKE_PARENT"
	// againParam names the build that a run runs again, whose parent and
	// root the new build keeps, even after the parent ended
	againParam = "WAKE_AGAIN"
)

// GroupsBucket holds a bucket for each root, with its members' IDs as keys,
// so a group is read without scanning the history
var GroupsBucket = []byte("groups")

// buildGroup is where a build sits in a group: both 0 for a build that no
// build started
type buildGroup struct {
	Parent int
	Root   int
}

// errNoParent is the answer for a parent that doesn't exist and for one the
// caller may not read, so the second can't be told from the first
func errNoParent(id int) error {
	return fmt.Errorf("there is no build %d", id)
}

// groupOf returns the group of a run of job from the request's WAKE_PARENT
// or WAKE_AGAIN. The parent must still be in the queue, so no one can hang
// builds under an old build and a chain can't loop. Run again keeps the
// group of the build it runs again, of the same job, so it can't hang other
// jobs under a group either. Both must be builds the caller may read.
func groupOf(r *http.Request, job string) (buildGroup, error) {
	parentValue, againValue := r.Form.Get(parentParam), r.Form.Get(againParam)
	if parentValue == "" && againValue == "" {
		return buildGroup{}, nil
	}
	if parentValue != "" && againValue != "" {
		return buildGroup{}, fmt.Errorf("send %s or %s, not both", parentParam, againParam)
	}
	access := accessFromContext(r.Context())
	if againValue != "" {
		id, err := runBuildID(againParam, againValue)
		if err != nil {
			return buildGroup{}, err
		}
		build, err := readBuildStatus(id)
		if errors.Is(err, errBuildNotFound) || (err == nil && !access.Reads(build.Name)) {
			return buildGroup{}, errNoParent(id)
		}
		if err != nil {
			return buildGroup{}, err
		}
		if build.Name != job {
			return buildGroup{}, fmt.Errorf("build %d is a build of %s, not of %s", id, build.Name, job)
		}
		return buildGroup{Parent: build.Parent, Root: build.Root}, nil
	}
	id, err := runBuildID(parentParam, parentValue)
	if err != nil {
		return buildGroup{}, err
	}
	parent := GlobalQueue.Live(id)
	if parent == nil {
		ended, err := readBuildStatus(id)
		if err == nil && access.Reads(ended.Name) {
			return buildGroup{}, fmt.Errorf("build %d has ended", id)
		}
		return buildGroup{}, errNoParent(id)
	}
	if !access.Reads(parent.Job.Name) {
		return buildGroup{}, errNoParent(id)
	}
	group := buildGroup{Parent: id, Root: parent.Group().Root}
	if group.Root == 0 {
		group.Root = id
	}
	return group, nil
}

// runBuildID reads the build ID that a run sends as name
func runBuildID(name, value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 || strconv.Itoa(id) != value {
		return 0, fmt.Errorf("%s must be a build ID, such as $WAKE_BUILD_ID", name)
	}
	return id, nil
}

// Group returns where the build sits in a group
func (b *Build) Group() buildGroup {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.group
}

// putGroupMember records build id as a member of its root's group
func putGroupMember(tx *bolt.Tx, root, id int) error {
	if root == 0 {
		return nil
	}
	groups, err := tx.CreateBucketIfNotExists(GroupsBucket)
	if err != nil {
		return err
	}
	members, err := groups.CreateBucketIfNotExists(Itob(root))
	if err != nil {
		return err
	}
	return members.Put(Itob(id), nil)
}

// groupMembers returns the IDs of the builds in root's group, in the order
// they were created, without root itself
func groupMembers(tx *bolt.Tx, root int) []int {
	groups := tx.Bucket(GroupsBucket)
	if groups == nil {
		return nil
	}
	members := groups.Bucket(Itob(root))
	if members == nil {
		return nil
	}
	var ids []int
	_ = members.ForEach(func(k, _ []byte) error {
		ids = append(ids, int(binary.BigEndian.Uint64(k)))
		return nil
	})
	return ids
}

// deleteGroupMember forgets build id, a member of root's group, and the
// group once it has no members
func deleteGroupMember(tx *bolt.Tx, root, id int) error {
	groups := tx.Bucket(GroupsBucket)
	if groups == nil || root == 0 {
		return nil
	}
	members := groups.Bucket(Itob(root))
	if members == nil {
		return nil
	}
	if err := members.Delete(Itob(id)); err != nil {
		return err
	}
	if k, _ := members.Cursor().First(); k == nil {
		return groups.DeleteBucket(Itob(root))
	}
	return nil
}

// GroupBuild is a build of a group, as the build page shows it
type GroupBuild struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Status ItemStatus `json:"status"`
	// Parent is the build that started it, 0 for the root
	Parent    int           `json:"parent"`
	QueuedAt  time.Time     `json:"queuedAt"`
	StartedAt time.Time     `json:"startedAt"`
	Duration  time.Duration `json:"duration" swaggertype:"integer"`
}

// GroupPayload is the group of a build
type GroupPayload struct {
	// Root is the first build of the group, 0 if the build is in none: no
	// build started it and it started none
	Root int `json:"root"`
	// Builds are the root, then the builds it and its members started, in
	// the order they were created, each one that the caller may read and
	// that cleanup kept
	Builds []GroupBuild `json:"builds"`
	// Hidden counts the members the caller may not read
	Hidden int `json:"hidden"`
}

// HandleBuildGroup returns the group of a build
// @Summary      Return the group of a build
// @Description  The builds started under the same first build with WAKE_PARENT, the root first. Empty for a build that no build started and that started none. API key scope: read
// @Tags         build
// @Produce      json
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {object}   GroupPayload
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      404      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id}/group [get]
func HandleBuildGroup(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := readGroup(id, accessFromContext(r.Context()))
	if errors.Is(err, errBuildNotFound) {
		writePlainText(w, logger, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		logger.Error("read build group", "build", id, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, logger, payload)
}

// readGroup reads the group of build id, with the builds access may read
func readGroup(id int, access *Access) (*GroupPayload, error) {
	payload := &GroupPayload{Builds: []GroupBuild{}}
	err := DB.View(func(tx *bolt.Tx) error {
		history := tx.Bucket(HistoryBucket)
		// read returns build id from the history and its root, nil if
		// cleanup removed it
		read := func(id int) (*GroupBuild, int, error) {
			data := history.Get(Itob(id))
			if data == nil {
				return nil, 0, nil
			}
			var build struct {
				GroupBuild
				Root int `json:"root"`
			}
			if err := json.Unmarshal(data, &build); err != nil {
				return nil, 0, fmt.Errorf("decode build %d: %w", id, err)
			}
			return &build.GroupBuild, build.Root, nil
		}
		build, root, err := read(id)
		if err != nil {
			return err
		}
		if build == nil {
			return errBuildNotFound
		}
		if root == 0 {
			root = id
		}
		members := groupMembers(tx, root)
		if len(members) == 0 {
			return nil
		}
		payload.Root = root
		for _, member := range append([]int{root}, members...) {
			build, _, err := read(member)
			if err != nil {
				return err
			}
			switch {
			case build == nil:
			case access.Reads(build.Name):
				payload.Builds = append(payload.Builds, *build)
			default:
				payload.Hidden++
			}
		}
		return nil
	})
	return payload, err
}
