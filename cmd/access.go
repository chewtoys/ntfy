//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func init() {
	commands = append(commands, cmdAccess)
}

const (
	userEveryone = "everyone"
)

var flagsAccess = append(
	append([]cli.Flag{}, flagsUser...),
	&cli.BoolFlag{Name: "reset", Aliases: []string{"r"}, Usage: "reset access for user (and topic)"},
)

var cmdAccess = &cli.Command{
	Name:      "access",
	Usage:     "Grant/revoke access to a topic, or show access",
	UsageText: "ntfy access [USERNAME [TOPIC [PERMISSION]]]",
	Flags:     flagsAccess,
	Before:    initConfigFileInputSourceFunc("config", flagsAccess, initLogFunc),
	Action:    execUserAccess,
	Category:  categoryServer,
	Description: `Manage the access control list for the ntfy server.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined. Please also refer
to the related command 'ntfy user'.

The command allows you to show the access control list, as well as change it, depending on how
it is called.

Usage:
  ntfy access                            # Shows access control list (alias: 'ntfy user list')
  ntfy access USERNAME                   # Shows access control entries for USERNAME
  ntfy access USERNAME TOPIC PERMISSION  # Allow/deny access for USERNAME to TOPIC

Arguments:
  USERNAME     an existing user, as created with 'ntfy user add', or "everyone"/"*"
               to define access rules for anonymous/unauthenticated clients
  TOPIC        name of a topic with optional wildcards, e.g. "mytopic*"
  PERMISSION   one of the following:
               - read-write (alias: rw) 
               - read-only (aliases: read, ro)
               - write-only (aliases: write, wo)
               - deny (alias: none)

Examples:
  ntfy access                        # Shows access control list (alias: 'ntfy user list')
  ntfy access phil                   # Shows access for user phil
  ntfy access phil mytopic rw        # Allow read-write access to mytopic for user phil
  ntfy access everyone mytopic rw    # Allow anonymous read-write access to mytopic
  ntfy access everyone "up*" write   # Allow anonymous write-only access to topics "up..." 
  ntfy access --reset                # Reset entire access control list
  ntfy access --reset phil           # Reset all access for user phil
  ntfy access --reset phil mytopic   # Reset access for user phil and topic mytopic
`,
}

func execUserAccess(c *cli.Context) error {
	if c.NArg() > 3 {
		return errors.New("too many arguments, please check 'ntfy access --help' for usage details")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	username := c.Args().Get(0)
	if username == userEveryone {
		username = user.Everyone
	}
	topic := c.Args().Get(1)
	perms := c.Args().Get(2)
	reset := c.Bool("reset")
	if reset {
		if perms != "" {
			return errors.New("too many arguments, please check 'ntfy access --help' for usage details")
		}
		return resetAccess(c, manager, username, topic)
	} else if perms == "" {
		if topic != "" {
			return errors.New("invalid syntax, please check 'ntfy access --help' for usage details")
		}
		return showAccess(c, manager, username)
	}
	return changeAccess(c, manager, username, topic, perms)
}

func changeAccess(c *cli.Context, manager *user.Manager, username string, topic string, perms string) error {
	if !util.Contains([]string{"", "read-write", "rw", "read-only", "read", "ro", "write-only", "write", "wo", "none", "deny"}, perms) {
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	}
	permission, err := user.ParsePermission(perms)
	if err != nil {
		return err
	}
	u, err := manager.User(username)
	if errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	} else if err != nil {
		return err
	} else if u.Role == user.RoleAdmin {
		return fmt.Errorf("user %s is an admin user, access control entries have no effect", username)
	}
	if err := manager.AllowAccess(username, topic, permission); err != nil {
		return err
	}
	if permission.IsReadWrite() {
		fmt.Fprintf(c.App.Writer, "granted read-write access to topic %s\n\n", topic)
	} else if permission.IsRead() {
		fmt.Fprintf(c.App.Writer, "granted read-only access to topic %s\n\n", topic)
	} else if permission.IsWrite() {
		fmt.Fprintf(c.App.Writer, "granted write-only access to topic %s\n\n", topic)
	} else {
		fmt.Fprintf(c.App.Writer, "revoked all access to topic %s\n\n", topic)
	}
	return showUserAccess(c, manager, username)
}

func resetAccess(c *cli.Context, manager *user.Manager, username, topic string) error {
	if username == "" {
		return resetAllAccess(c, manager)
	} else if topic == "" {
		return resetUserAccess(c, manager, username)
	}
	return resetUserTopicAccess(c, manager, username, topic)
}

func resetAllAccess(c *cli.Context, manager *user.Manager) error {
	if err := manager.ResetAccess("", ""); err != nil {
		return err
	}
	fmt.Fprintln(c.App.Writer, "reset access for all users")
	return nil
}

func resetUserAccess(c *cli.Context, manager *user.Manager, username string) error {
	if err := manager.ResetAccess(username, ""); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "reset access for user %s\n\n", username)
	return showUserAccess(c, manager, username)
}

func resetUserTopicAccess(c *cli.Context, manager *user.Manager, username string, topic string) error {
	if err := manager.ResetAccess(username, topic); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "reset access for user %s and topic %s\n\n", username, topic)
	return showUserAccess(c, manager, username)
}

func showAccess(c *cli.Context, manager *user.Manager, username string) error {
	if username == "" {
		return showAllAccess(c, manager)
	}
	return showUserAccess(c, manager, username)
}

func showAllAccess(c *cli.Context, manager *user.Manager) error {
	users, err := manager.Users()
	if err != nil {
		return err
	}
	return showUsers(c, manager, users)
}

func showUserAccess(c *cli.Context, manager *user.Manager, username string) error {
	users, err := manager.User(username)
	if errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	} else if err != nil {
		return err
	}
	return showUsers(c, manager, []*user.User{users})
}

func showUsers(c *cli.Context, manager *user.Manager, users []*user.User) error {
	for _, u := range users {
		grants, err := manager.Grants(u.Name)
		if err != nil {
			return err
		}
		tier := "none"
		if u.Tier != nil {
			tier = u.Tier.Name
		}
		provisioned := ""
		if u.Provisioned {
			provisioned = ", server config"
		}
		fmt.Fprintf(c.App.Writer, "user %s (role: %s, tier: %s%s)\n", u.Name, u.Role, tier, provisioned)
		if u.Role == user.RoleAdmin {
			fmt.Fprintf(c.App.Writer, "- read-write access to all topics (admin role)\n")
		} else if len(grants) > 0 {
			for _, grant := range grants {
				grantProvisioned := ""
				if grant.Provisioned {
					grantProvisioned = " (server config)"
				}
				if grant.Permission.IsReadWrite() {
					fmt.Fprintf(c.App.Writer, "- read-write access to topic %s%s\n", grant.TopicPattern, grantProvisioned)
				} else if grant.Permission.IsRead() {
					fmt.Fprintf(c.App.Writer, "- read-only access to topic %s%s\n", grant.TopicPattern, grantProvisioned)
				} else if grant.Permission.IsWrite() {
					fmt.Fprintf(c.App.Writer, "- write-only access to topic %s%s\n", grant.TopicPattern, grantProvisioned)
				} else {
					fmt.Fprintf(c.App.Writer, "- no access to topic %s%s\n", grant.TopicPattern, grantProvisioned)
				}
			}
		} else {
			fmt.Fprintf(c.App.Writer, "- no topic-specific permissions\n")
		}
		if u.Name == user.Everyone {
			access := manager.DefaultAccess()
			if access.IsReadWrite() {
				fmt.Fprintln(c.App.Writer, "- read-write access to all (other) topics (server config)")
			} else if access.IsRead() {
				fmt.Fprintln(c.App.Writer, "- read-only access to all (other) topics (server config)")
			} else if access.IsWrite() {
				fmt.Fprintln(c.App.Writer, "- write-only access to all (other) topics (server config)")
			} else {
				fmt.Fprintln(c.App.Writer, "- no access to any (other) topics (server config)")
			}
		}
	}
	return nil
}
