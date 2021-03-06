package commands

import (
	"fmt"

	"github.com/camd67/moebot/moebot_bot/util/db"
)

type ChangelogCommand struct {
	Version string
}

const changeLogPrefix = "\n`->` "

// probably want to move this to the DB, but not bad to have it here

////////////////////////////////////////////////////////////////////
//   Please only edit this in develop before merging to master    //
////////////////////////////////////////////////////////////////////
var changeLog = map[string]string{

	"0.4.2": changeLogPrefix + "Improved profile command" +
		changeLogPrefix + "Fix bug with permission checking" +
		changeLogPrefix + "Fix bug with help command displaying too much" +
		changeLogPrefix + "Change pinmove to use different infrastructure, and include delete option" +
		changeLogPrefix + "(0.4.2) Fix permit bug not assigning a permission value",

	"0.4.0": changeLogPrefix + "Removed all hardcoded values!" +
		changeLogPrefix + "Merged rank, team, and NSFW into a single command: role" +
		changeLogPrefix + "Added roleSet and groupSet commands for mods" +
		changeLogPrefix + "Added server configuration for welcome messages, and rule agreement as well as server config clearing" +
		changeLogPrefix + "Made help contextual to your permission level (Credit: Shadran)" +
		changeLogPrefix + "First big step towards public bot status!",

	"0.3.2": changeLogPrefix + "More code cleanup" +
		changeLogPrefix + "Removed spoiler command (thanks discord)" +
		changeLogPrefix + "Added verifiable roles (Credit: Shadran)",

	"0.3.1": changeLogPrefix + "Code cleanup/refactor" +
		changeLogPrefix + "Updated veteran rank handling",

	"0.3": changeLogPrefix + "Added veteran role stuff" +
		changeLogPrefix + "Added pinmove command (credit: Shadran)" +
		changeLogPrefix + "Added server configuration for mods" +
		changeLogPrefix + "Added the profile command" +
		changeLogPrefix + "Fixed some bugs",

	"0.2.4": changeLogPrefix + "Added spoiler command (credit: Shadran)" +
		changeLogPrefix + "Added poll command (credit: Shadran)",

	"0.2.3": changeLogPrefix + "Added ping command" +
		changeLogPrefix + "Added permit command" +
		changeLogPrefix + "Added custom command",

	"0.2.2": changeLogPrefix + "Added echo command for master only" +
		changeLogPrefix + "added `raffle winner` and `raffle count` to get the raffle winner and vote counts" +
		changeLogPrefix + "removed ticket generation",

	"0.2.1": changeLogPrefix + "Updated raffle art/relic submissions to post all submissions on command instead of over time.",

	"0.2": changeLogPrefix + "Included this command!" +
		changeLogPrefix + "Updated `Rank` command to prevent removal of lowest role." +
		changeLogPrefix + "Added random drops for tickets" +
		changeLogPrefix + "Fixed the cooldown so users wouldn't be spammed due to high luck stat" +
		changeLogPrefix + "Added `Raffle` related commands... For rafflin'" +
		changeLogPrefix + "For future reference, previous versions included help, team, rank, and NSFW commands as well as a welcome message to the server.",
}

func (cc *ChangelogCommand) Execute(pack *CommPackage) {
	if len(pack.params) == 0 {
		pack.session.ChannelMessageSend(pack.channel.ID, "Moebot update log `(ver "+cc.Version+")`: \n"+changeLog[cc.Version])
	} else if log, present := changeLog[pack.params[0]]; present {
		pack.session.ChannelMessageSend(pack.channel.ID, "Moebot update log `(ver "+pack.params[0]+")`: \n"+log)
	} else {
		pack.session.ChannelMessageSend(pack.channel.ID, "Unknown version number. Latest log:\nMoebot update log `(ver "+cc.Version+")`: \n"+changeLog[cc.Version])
	}
}

func (cc *ChangelogCommand) GetPermLevel() db.Permission {
	return db.PermAll
}

func (cc *ChangelogCommand) GetCommandKeys() []string {
	return []string{"CHANGELOG"}
}

func (cc *ChangelogCommand) GetCommandHelp(commPrefix string) string {
	return fmt.Sprintf("`%[1]s changelog` - Displays the changelog for moebot", commPrefix)
}
