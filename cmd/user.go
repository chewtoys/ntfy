//go:build !noserver

package cmd

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/user"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/util"
)

const (
	tierReset = "-"
)

func init() {
	commands = append(commands, cmdUser)
}

var flagsUser = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: server.DefaultConfigFile, DefaultText: server.DefaultConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file", "H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file used for access control"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-default-access", Aliases: []string{"auth_default_access", "p"}, EnvVars: []string{"NTFY_AUTH_DEFAULT_ACCESS"}, Value: "read-write", Usage: "default permissions if no matching entries in the auth database are found"}),
)

var cmdUser = &cli.Command{
	Name:      "user",
	Usage:     "Manage/show users",
	UsageText: "ntfy user [list|add|remove|change-pass|change-role] ...",
	Flags:     flagsUser,
	Before:    initConfigFileInputSourceFunc("config", flagsUser, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "add",
			Aliases:   []string{"a"},
			Usage:     "Adds a new user",
			UsageText: "ntfy user add [--role=admin|user] USERNAME\nNTFY_PASSWORD=... ntfy user add [--role=admin|user] USERNAME\nNTFY_PASSWORD_HASH=... ntfy user add [--role=admin|user] USERNAME",
			Action:    execUserAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "role", Aliases: []string{"r"}, Value: string(user.RoleUser), Usage: "user role"},
				&cli.BoolFlag{Name: "ignore-exists", Usage: "if the user already exists, perform no action and exit"},
			},
			Description: `Add a new user to the ntfy user database.

A user can be either a regular user, or an admin. A regular user has no read or write access (unless
granted otherwise by the auth-default-access setting). An admin user has read and write access to all
topics.

Examples:
  ntfy user add phil                          # Add regular user phil
  ntfy user add --role=admin phil             # Add admin user phil
  NTFY_PASSWORD=... ntfy user add phil        # Add user, using env variable to set password (for scripts)
  NTFY_PASSWORD_HASH=... ntfy user add phil   # Add user, using env variable to set password hash (for scripts)

You may set the NTFY_PASSWORD environment variable to pass the password, or NTFY_PASSWORD_HASH to pass
directly the bcrypt hash. This is useful if you are creating users via scripts.
`,
		},
		{
			Name:      "remove",
			Aliases:   []string{"del", "rm"},
			Usage:     "Removes a user",
			UsageText: "ntfy user remove USERNAME",
			Action:    execUserDel,
			Description: `Remove a user from the ntfy user database.

Example:
  ntfy user del phil
`,
		},
		{
			Name:      "change-pass",
			Aliases:   []string{"chp"},
			Usage:     "Changes a user's password",
			UsageText: "ntfy user change-pass USERNAME\nNTFY_PASSWORD=... ntfy user change-pass USERNAME\nNTFY_PASSWORD_HASH=... ntfy user change-pass USERNAME",
			Action:    execUserChangePass,
			Description: `Change the password for the given user.

The new password will be read from STDIN, and it'll be confirmed by typing
it twice. 

Example:
  ntfy user change-pass phil
  NTFY_PASSWORD=.. ntfy user change-pass phil
  NTFY_PASSWORD_HASH=.. ntfy user change-pass phil

You may set the NTFY_PASSWORD environment variable to pass the new password or NTFY_PASSWORD_HASH to pass
directly the bcrypt hash. This is useful if you are updating users via scripts.
`,
		},
		{
			Name:      "change-role",
			Aliases:   []string{"chr"},
			Usage:     "Changes the role of a user",
			UsageText: "ntfy user change-role USERNAME ROLE",
			Action:    execUserChangeRole,
			Description: `Change the role for the given user to admin or user.

This command can be used to change the role of a user either from a regular user
to an admin user, or the other way around:

- admin: an admin has read/write access to all topics
- user: a regular user only has access to what was explicitly granted via 'ntfy access'

When changing the role of a user to "admin", all access control entries for that 
user are removed, since they are no longer necessary.

Example:
  ntfy user change-role phil admin   # Make user phil an admin 
  ntfy user change-role phil user    # Remove admin role from user phil 
`,
		},
		{
			Name:      "change-tier",
			Aliases:   []string{"cht"},
			Usage:     "Changes the tier of a user",
			UsageText: "ntfy user change-tier USERNAME (TIER|-)",
			Action:    execUserChangeTier,
			Description: `Change the tier for the given user.

This command can be used to change the tier of a user. Tiers define usage limits, such
as messages per day, attachment file sizes, etc.

Example:
  ntfy user change-tier phil pro   # Change tier to "pro" for user "phil"  
  ntfy user change-tier phil -     # Remove tier from user "phil" entirely 
`,
		},
		{
			Name:      "hash",
			Usage:     "Create password hash for a predefined user",
			UsageText: "ntfy user hash",
			Action:    execUserHash,
			Description: `Asks for a password and creates a bcrypt password hash.

This command is useful to create a password hash for a user, which can then be used
for predefined users in the server config file, in auth-users.

Example:
  $ ntfy user hash
  (asks for password and confirmation)
  $2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C
`,
		},
		{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "Shows a list of users",
			Action:  execUserList,
			Description: `Shows a list of all configured users, including the everyone ('*') user.

This command is an alias to calling 'ntfy access' (display access control list).

This is a server-only command. It directly reads from user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.
`,
		},
	},
	Description: `Manage users of the ntfy server.

The command allows you to add/remove/change users in the ntfy user database, as well as change 
passwords or roles.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined. Please also refer
to the related command 'ntfy access'.

Examples:
  ntfy user list                               # Shows list of users (alias: 'ntfy access')                      
  ntfy user add phil                           # Add regular user phil  
  NTFY_PASSWORD=... ntfy user add phil         # As above, using env variable to set password (for scripts)
  ntfy user add --role=admin phil              # Add admin user phil
  ntfy user del phil                           # Delete user phil
  ntfy user change-pass phil                   # Change password for user phil
  NTFY_PASSWORD=.. ntfy user change-pass phil  # As above, using env variable to set password (for scripts)
  ntfy user change-role phil admin             # Make user phil an admin 

For the 'ntfy user add' and 'ntfy user change-pass' commands, you may set the NTFY_PASSWORD environment
variable to pass the new password. This is useful if you are creating/updating users via scripts.
`,
}

func execUserAdd(c *cli.Context) error {
	username := c.Args().Get(0)
	role := user.Role(c.String("role"))
	password, hashed := os.LookupEnv("NTFY_PASSWORD_HASH")

	if !hashed {
		password = os.Getenv("NTFY_PASSWORD")
	}

	if username == "" {
		return errors.New("username expected, type 'ntfy user add --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	} else if !user.AllowedRole(role) {
		return errors.New("role must be either 'user' or 'admin'")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if user, _ := manager.User(username); user != nil {
		if c.Bool("ignore-exists") {
			fmt.Fprintf(c.App.Writer, "user %s already exists (exited successfully)\n", username)
			return nil
		}
		return fmt.Errorf("user %s already exists", username)
	}
	if password == "" {
		p, err := readPasswordAndConfirm(c)
		if err != nil {
			return err
		}
		password = p
	}
	if err := manager.AddUser(username, password, role, hashed); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "user %s added with role %s\n", username, role)
	return nil
}

func execUserDel(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == "" {
		return errors.New("username expected, type 'ntfy user del --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	}
	if err := manager.RemoveUser(username); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "user %s removed\n", username)
	return nil
}

func execUserChangePass(c *cli.Context) error {
	username := c.Args().Get(0)
	password, hashed := os.LookupEnv("NTFY_PASSWORD_HASH")

	if !hashed {
		password = os.Getenv("NTFY_PASSWORD")
	}
	if username == "" {
		return errors.New("username expected, type 'ntfy user change-pass --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	}
	if password == "" {
		password, err = readPasswordAndConfirm(c)
		if err != nil {
			return err
		}
	}
	if err := manager.ChangePassword(username, password, hashed); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "changed password for user %s\n", username)
	return nil
}

func execUserChangeRole(c *cli.Context) error {
	username := c.Args().Get(0)
	role := user.Role(c.Args().Get(1))
	if username == "" || !user.AllowedRole(role) {
		return errors.New("username and new role expected, type 'ntfy user change-role --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	}
	if err := manager.ChangeRole(username, role); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "changed role for user %s to %s\n", username, role)
	return nil
}

func execUserHash(c *cli.Context) error {
	password, err := readPasswordAndConfirm(c)
	if err != nil {
		return err
	}
	hash, err := user.HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	fmt.Fprintln(c.App.Writer, hash)
	return nil
}

func execUserChangeTier(c *cli.Context) error {
	username := c.Args().Get(0)
	tier := c.Args().Get(1)
	if username == "" {
		return errors.New("username and new tier expected, type 'ntfy user change-tier --help' for help")
	} else if !user.AllowedTier(tier) && tier != tierReset {
		return errors.New("invalid tier, must be tier code, or - to reset")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); errors.Is(err, user.ErrUserNotFound) {
		return fmt.Errorf("user %s does not exist", username)
	}
	if tier == tierReset {
		if err := manager.ResetTier(username); err != nil {
			return err
		}
		fmt.Fprintf(c.App.Writer, "removed tier from user %s\n", username)
	} else {
		if err := manager.ChangeTier(username, tier); err != nil {
			return err
		}
		fmt.Fprintf(c.App.Writer, "changed tier for user %s to %s\n", username, tier)
	}
	return nil
}

func execUserList(c *cli.Context) error {
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	users, err := manager.Users()
	if err != nil {
		return err
	}
	return showUsers(c, manager, users)
}

func createUserManager(c *cli.Context) (*user.Manager, error) {
	authFile := c.String("auth-file")
	authStartupQueries := c.String("auth-startup-queries")
	authDefaultAccess := c.String("auth-default-access")
	if authFile == "" {
		return nil, errors.New("option auth-file not set; auth is unconfigured for this server")
	} else if !util.FileExists(authFile) {
		return nil, errors.New("auth-file does not exist; please start the server at least once to create it")
	}
	authDefault, err := user.ParsePermission(authDefaultAccess)
	if err != nil {
		return nil, errors.New("if set, auth-default-access must start set to 'read-write', 'read-only', 'write-only' or 'deny-all'")
	}
	authConfig := &user.Config{
		Filename:            authFile,
		StartupQueries:      authStartupQueries,
		DefaultAccess:       authDefault,
		ProvisionEnabled:    false, // Hack: Do not re-provision users on manager initialization
		BcryptCost:          user.DefaultUserPasswordBcryptCost,
		QueueWriterInterval: user.DefaultUserStatsQueueWriterInterval,
	}
	return user.NewManager(authConfig)
}

func readPasswordAndConfirm(c *cli.Context) (string, error) {
	fmt.Fprint(c.App.ErrWriter, "password: ")
	password, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
	} else if len(password) == 0 {
		return "", errors.New("password cannot be empty")
	}
	fmt.Fprintf(c.App.ErrWriter, "\r%s\rconfirm: ", strings.Repeat(" ", 25))
	confirm, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(c.App.ErrWriter, "\r%s\r", strings.Repeat(" ", 25))
	if subtle.ConstantTimeCompare(confirm, password) != 1 {
		return "", errors.New("passwords do not match: try it again, but this time type slooowwwlly")
	}
	return string(password), nil
}
