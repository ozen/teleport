//go:build linux
// +build linux

/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/backend/lite"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/services/local"
	"github.com/gravitational/teleport/lib/srv"
	"github.com/gravitational/teleport/lib/utils/host"
)

const testuser = "teleport-testuser"
const testgroup = "teleport-testgroup"

func requireRoot(t *testing.T) {
	t.Helper()
	if !isRoot() {
		t.Skip("This test will be skipped because tests are not being run as root.")
	}
}

func TestRootHostUsersBackend(t *testing.T) {
	requireRoot(t)
	sudoersTestDir := t.TempDir()
	usersbk := srv.HostUsersProvisioningBackend{}
	sudoersbk := srv.HostSudoersProvisioningBackend{
		SudoersPath: sudoersTestDir,
		HostUUID:    "hostuuid",
	}
	t.Cleanup(func() {
		// cleanup users if they got left behind due to a failing test
		host.UserDel(testuser)
		cmd := exec.Command("groupdel", testgroup)
		cmd.Run()
	})

	t.Run("Test CreateGroup", func(t *testing.T) {
		err := usersbk.CreateGroup(testgroup, "")
		require.NoError(t, err)
		err = usersbk.CreateGroup(testgroup, "")
		require.True(t, trace.IsAlreadyExists(err))
	})

	t.Run("Test CreateUser and group", func(t *testing.T) {
		err := usersbk.CreateUser(testuser, []string{testgroup}, "", "")
		require.NoError(t, err)

		tuser, err := usersbk.Lookup(testuser)
		require.NoError(t, err)

		group, err := usersbk.LookupGroup(testgroup)
		require.NoError(t, err)

		tuserGids, err := tuser.GroupIds()
		require.NoError(t, err)
		require.Contains(t, tuserGids, group.Gid)

		err = usersbk.CreateUser(testuser, []string{}, "", "")
		require.True(t, trace.IsAlreadyExists(err))

		require.NoFileExists(t, filepath.Join("/home", testuser))
		err = usersbk.CreateHomeDirectory(testuser, tuser.Uid, tuser.Gid)
		require.NoError(t, err)
		require.FileExists(t, filepath.Join("/home", testuser, ".bashrc"))
	})

	t.Run("Test DeleteUser", func(t *testing.T) {
		err := usersbk.DeleteUser(testuser)
		require.NoError(t, err)

		_, err = user.Lookup(testuser)
		require.Equal(t, err, user.UnknownUserError(testuser))
	})

	t.Run("Test GetAllUsers", func(t *testing.T) {
		checkUsers := []string{"teleport-usera", "teleport-userb", "teleport-userc"}
		t.Cleanup(func() {
			for _, u := range checkUsers {
				usersbk.DeleteUser(u)
			}
		})
		for _, u := range checkUsers {
			err := usersbk.CreateUser(u, []string{}, "", "")
			require.NoError(t, err)
		}

		users, err := usersbk.GetAllUsers()
		require.NoError(t, err)
		require.Subset(t, users, append(checkUsers, "root"))
	})

	t.Run("Test sudoers", func(t *testing.T) {
		if _, err := exec.LookPath("visudo"); err != nil {
			t.Skip("visudo not found on path")
		}
		validSudoersEntry := []byte("root ALL=(ALL) ALL")
		err := sudoersbk.CheckSudoers(validSudoersEntry)
		require.NoError(t, err)
		invalidSudoersEntry := []byte("yipee i broke sudo!!!!")
		err = sudoersbk.CheckSudoers(invalidSudoersEntry)
		require.Contains(t, err.Error(), "visudo: invalid sudoers file")
		// test sudoers entry containing . or ~
		require.NoError(t, sudoersbk.WriteSudoersFile("user.name", validSudoersEntry))
		_, err = os.Stat(filepath.Join(sudoersTestDir, "teleport-hostuuid-user_name"))
		require.NoError(t, err)
		require.NoError(t, sudoersbk.RemoveSudoersFile("user.name"))
		_, err = os.Stat(filepath.Join(sudoersTestDir, "teleport-hostuuid-user_name"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Test CreateHomeDirectory does not follow symlinks", func(t *testing.T) {
		err := usersbk.CreateUser(testuser, []string{testgroup}, "", "")
		require.NoError(t, err)

		tuser, err := usersbk.Lookup(testuser)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile("/etc/skel/testfile", []byte("test\n"), 0o700))

		bashrcPath := filepath.Join("/home", testuser, ".bashrc")
		require.NoFileExists(t, bashrcPath)

		require.NoError(t, os.MkdirAll(filepath.Join("/home", testuser), 0o700))

		require.NoError(t, os.Symlink("/tmp/ignoreme", bashrcPath))
		require.NoFileExists(t, "/tmp/ignoreme")

		err = usersbk.CreateHomeDirectory(testuser, tuser.Uid, tuser.Gid)
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join("/home", testuser))
		})
		require.NoError(t, err)
		require.NoFileExists(t, "/tmp/ignoreme")
	})
}

func requireUserInGroups(t *testing.T, u *user.User, requiredGroups []string) {
	var userGroups []string
	userGids, err := u.GroupIds()
	require.NoError(t, err)
	for _, gid := range userGids {
		group, err := user.LookupGroupId(gid)
		require.NoError(t, err)
		userGroups = append(userGroups, group.Name)
	}
	require.Subset(t, userGroups, requiredGroups)
}

func cleanupUsersAndGroups(users []string, groups []string) func() {
	return func() {
		for _, group := range groups {
			cmd := exec.Command("groupdel", group)
			err := cmd.Run()
			if err != nil {
				log.Debugf("Error deleting group %s: %s", group, err)
			}
		}
		for _, user := range users {
			host.UserDel(user)
		}
	}
}

func TestRootHostUsers(t *testing.T) {
	requireRoot(t)
	ctx := context.Background()
	bk, err := lite.New(ctx, backend.Params{"path": t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bk.Close()) })
	presence := local.NewPresenceService(bk)

	t.Run("test create temporary user and close", func(t *testing.T) {
		users := srv.NewHostUsers(context.Background(), presence, "host_uuid")

		testGroups := []string{"group1", "group2"}
		closer, err := users.CreateUser(testuser, &services.HostUsersInfo{Groups: testGroups, Mode: types.CreateHostUserMode_HOST_USER_MODE_DROP})
		require.NoError(t, err)

		testGroups = append(testGroups, types.TeleportServiceGroup)
		t.Cleanup(cleanupUsersAndGroups([]string{testuser}, testGroups))

		u, err := user.Lookup(testuser)
		require.NoError(t, err)
		requireUserInGroups(t, u, testGroups)
		require.NotEmpty(t, u.HomeDir)
		require.DirExists(t, u.HomeDir)

		require.NoError(t, closer.Close())
		_, err = user.Lookup(testuser)
		require.Equal(t, err, user.UnknownUserError(testuser))
		require.NoDirExists(t, u.HomeDir)
	})

	t.Run("test create temporary user without home dir", func(t *testing.T) {
		users := srv.NewHostUsers(context.Background(), presence, "host_uuid")

		testGroups := []string{"group1", "group2"}
		closer, err := users.CreateUser(testuser, &services.HostUsersInfo{Groups: testGroups, Mode: types.CreateHostUserMode_HOST_USER_MODE_INSECURE_DROP})
		require.NoError(t, err)

		testGroups = append(testGroups, types.TeleportServiceGroup)
		t.Cleanup(cleanupUsersAndGroups([]string{testuser}, testGroups))

		u, err := user.Lookup(testuser)
		require.NoError(t, err)
		requireUserInGroups(t, u, testGroups)
		require.NoDirExists(t, u.HomeDir)

		require.NoError(t, closer.Close())
		_, err = user.Lookup(testuser)
		require.Equal(t, err, user.UnknownUserError(testuser))
	})

	t.Run("test create user with uid and gid", func(t *testing.T) {
		users := srv.NewHostUsers(context.Background(), presence, "host_uuid")

		testUID := "1234"
		testGID := "1337"

		_, err := user.LookupGroupId(testGID)
		require.ErrorIs(t, err, user.UnknownGroupIdError(testGID))

		closer, err := users.CreateUser(testuser, &services.HostUsersInfo{
			Mode: types.CreateHostUserMode_HOST_USER_MODE_DROP,
			UID:  testUID,
			GID:  testGID,
		})
		require.NoError(t, err)

		t.Cleanup(cleanupUsersAndGroups([]string{testuser}, []string{types.TeleportServiceGroup}))

		group, err := user.LookupGroupId(testGID)
		require.NoError(t, err)
		require.Equal(t, testuser, group.Name)

		u, err := user.Lookup(testuser)
		require.NoError(t, err)

		require.Equal(t, u.Uid, testUID)
		require.Equal(t, u.Gid, testGID)

		require.FileExists(t, filepath.Join("/home", testuser, ".bashrc"))

		require.NoError(t, closer.Close())
		_, err = user.Lookup(testuser)
		require.Equal(t, err, user.UnknownUserError(testuser))
	})

	t.Run("test create sudoers enabled users", func(t *testing.T) {
		if _, err := exec.LookPath("visudo"); err != nil {
			t.Skip("Visudo not found on path")
		}
		uuid := "host_uuid"
		users := srv.NewHostUsers(context.Background(), presence, uuid)
		sudoers := srv.NewHostSudoers(uuid)

		sudoersPath := func(username, uuid string) string {
			return fmt.Sprintf("/etc/sudoers.d/teleport-%s-%s", uuid, username)
		}

		t.Cleanup(func() {
			os.Remove(sudoersPath(testuser, uuid))
			host.UserDel(testuser)
		})
		closer, err := users.CreateUser(testuser,
			&services.HostUsersInfo{
				Mode: types.CreateHostUserMode_HOST_USER_MODE_DROP,
			})
		require.NoError(t, err)
		err = sudoers.WriteSudoers(testuser, []string{"ALL=(ALL) ALL"})
		require.NoError(t, err)
		_, err = os.Stat(sudoersPath(testuser, uuid))
		require.NoError(t, err)

		// delete the user and ensure the sudoers file got deleted
		require.NoError(t, closer.Close())
		require.NoError(t, sudoers.RemoveSudoers(testuser))
		_, err = os.Stat(sudoersPath(testuser, uuid))
		require.True(t, os.IsNotExist(err))

		// ensure invalid sudoers entries dont get written
		err = sudoers.WriteSudoers(testuser,
			[]string{"badsudoers entry!!!"},
		)
		require.Error(t, err)
		_, err = os.Stat(sudoersPath(testuser, uuid))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("test delete all users in teleport service group", func(t *testing.T) {
		users := srv.NewHostUsers(context.Background(), presence, "host_uuid")
		users.SetHostUserDeletionGrace(0)

		deleteableUsers := []string{"teleport-user1", "teleport-user2", "teleport-user3"}
		for _, user := range deleteableUsers {
			_, err := users.CreateUser(user, &services.HostUsersInfo{Mode: types.CreateHostUserMode_HOST_USER_MODE_DROP})
			require.NoError(t, err)
		}

		// this user should not be in the service group as it was created with mode keep.
		closer, err := users.CreateUser("teleport-user4", &services.HostUsersInfo{
			Mode: types.CreateHostUserMode_HOST_USER_MODE_KEEP,
		})
		require.NoError(t, err)
		require.Nil(t, closer)

		t.Cleanup(cleanupUsersAndGroups(
			[]string{"teleport-user1", "teleport-user2", "teleport-user3", "teleport-user4"},
			[]string{types.TeleportServiceGroup}))

		err = users.DeleteAllUsers()
		require.NoError(t, err)

		_, err = user.Lookup("teleport-user4")
		require.NoError(t, err)

		for _, us := range deleteableUsers {
			_, err := user.Lookup(us)
			require.Equal(t, err, user.UnknownUserError(us))
		}
	})
}
