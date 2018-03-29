package bot

import (
	"log"
	"strings"

	"github.com/camd67/moebot/moebot_bot/bot/commands"
	"github.com/camd67/moebot/moebot_bot/bot/permissions"
	"github.com/camd67/moebot/moebot_bot/util"
	"github.com/camd67/moebot/moebot_bot/util/db"

	"github.com/bwmarrin/discordgo"
)

const (
	version = "0.3.2"
)

var (
	// need to move this to the db
	allowedNonComServers = []string{"378336255030722570", "93799773856862208"}
	ComPrefix            string
	Config               = make(map[string]string)
	operations           []interface{}
	commandsMap          = make(map[string]commands.Command)
	checker              permissions.PermissionChecker
	masterId             string
)

/**
Run through initial setup steps for Moebot. This is all that's necessary to setup Moebot for use
*/
func SetupMoebot(session *discordgo.Session) {
	masterId = Config["masterId"]
	db.SetupDatabase(Config["dbPass"], Config["moeDataPass"])
	addGlobalHandlers(session)
	setupOperations(session)
	checker = permissions.PermissionChecker{MasterId: masterId}
}

/**
Create all the operations to handle commands and events within moebot.
Whenever a new operation, command, or event is added it should be added to this list
*/
func setupOperations(session *discordgo.Session) {
	roleHandler := &commands.RoleHandler{ComPrefix: ComPrefix}
	operations = []interface{}{
		&commands.TeamCommand{Handler: roleHandler},
		&commands.RoleCommand{},
		&commands.RankCommand{Handler: roleHandler},
		&commands.NsfwCommand{Handler: roleHandler},
		&commands.HelpCommand{ComPrefix: ComPrefix, CommandsMap: commandsMap, Checker: checker},
		&commands.ChangelogCommand{Version: version},
		&commands.RaffleCommand{MasterId: masterId, DebugChannel: Config["debugChannel"]},
		&commands.SubmitCommand{ComPrefix: ComPrefix},
		&commands.EchoCommand{},
		&commands.PermitCommand{},
		&commands.CustomCommand{ComPrefix: ComPrefix},
		&commands.PingCommand{},
		&commands.SpoilerCommand{},
		&commands.PollCommand{PollsHandler: commands.NewPollsHandler()},
		&commands.MentionCommand{},
		&commands.ServerCommand{},
		&commands.ProfileCommand{MasterId: masterId},
		&commands.PinMoveCommand{ShouldLoadPins: Config["loadPins"] == "1"},
		commands.NewVeteranHandler(ComPrefix, Config["debugChannel"], masterId),
	}

	setupCommands()
	setupHandlers(session)
	setupEvents(session)
}

/**
Run through each operation and place each command into the command map (including any aliases)
*/
func setupCommands() {
	for _, o := range operations {
		if command, ok := o.(commands.Command); ok {
			for _, key := range command.GetCommandKeys() {
				commandsMap[key] = command
			}
		}
	}
}

/**
Run through each operation and run through any setup steps required by those operations
*/
func setupHandlers(session *discordgo.Session) {
	for _, o := range operations {
		if setup, ok := o.(commands.SetupHandler); ok {
			setup.Setup(session)
		}
	}
}

/**
Run through each operation and add a handler for any discord events those operations need
*/
func setupEvents(session *discordgo.Session) {
	for _, o := range operations {
		if handler, ok := o.(commands.EventHandler); ok {
			for _, h := range handler.EventHandlers() {
				session.AddHandler(h)
			}
		}
	}
}

/**
These handlers are global for all of moebot such as message creation and ready
*/
func addGlobalHandlers(discord *discordgo.Session) {
	discord.AddHandler(ready)
	discord.AddHandler(messageCreate)
	discord.AddHandler(guildMemberAdd)
}

/**
Global handler for when new guild members join a discord guild. Typically used to welcome them if the server has enabled it.
*/
func guildMemberAdd(session *discordgo.Session, member *discordgo.GuildMemberAdd) {
	guild, err := session.Guild(member.GuildID)
	// temp ignore IHG + Salt
	if guild.ID == "84724034129907712" || guild.ID == "93799773856862208" {
		return
	}
	if err != nil {
		log.Println("Error when getting guild for user when joining guild", member)
		return
	}

	var starterRole *discordgo.Role
	for _, guildRole := range guild.Roles {
		if guildRole.Name == "Bell" {
			starterRole = guildRole
			break
		}
	}
	if starterRole == nil {
		log.Println("ERROR! Unable to find starter role for guild " + guild.Name)
		return
	}
	dmChannel, err := session.UserChannelCreate(member.User.ID)
	if err != nil {
		log.Println("ERROR! Unable to make DM channel with userID ", member.User.ID)
		return
	}
	session.ChannelMessageSend(dmChannel.ID, "Hello "+member.User.Mention()+" and welcome to "+guild.Name+"! Please read the #rules channel for more information")
	session.GuildMemberRoleAdd(member.GuildID, member.User.ID, starterRole.ID)
}

/**
Global handler for when new messages are sent in any guild. The entry point for commands and other general handling
*/
func messageCreate(session *discordgo.Session, message *discordgo.MessageCreate) {
	// bail out if we have any messages we want to ignore such as bot messages
	if message.Author.ID == session.State.User.ID || message.Author.Bot {
		return
	}

	channel, err := session.State.Channel(message.ChannelID)
	if err != nil {
		// missing channel
		log.Println("ERROR! Unable to get guild in messageCreate ", err, channel)
		return
	}

	guild, err := session.Guild(channel.GuildID)
	if err != nil {
		log.Println("ERROR! Unable to get guild in messageCreate ", err, guild)
		return
	}

	member, err := session.GuildMember(guild.ID, message.Author.ID)
	if err != nil {
		log.Println("ERROR! Unable to get member in messageCreate ", err, message)
		return
	}
	// temp ignore IHG
	if err != nil || guild.ID == "84724034129907712" {
		// missing guild
		return
	}
	// should change this to store the ID of the starting role
	var starterRole *discordgo.Role
	var oldStarterRole *discordgo.Role
	for _, guildRole := range guild.Roles {
		if strings.EqualFold(guildRole.Name, "Red Whistle") {
			starterRole = guildRole
		} else if strings.EqualFold(guildRole.Name, "Bell") {
			oldStarterRole = guildRole
		}
	}

	if strings.HasPrefix(message.Content, ComPrefix) {
		// should add a check here for command spam
		if oldStarterRole != nil && util.StrContains(member.Roles, oldStarterRole.ID, util.CaseSensitive) {
			// bail out to prevent any new users from using bot commands
			return
		}
		runCommand(session, message.Message, guild, channel, member)
	} else if util.StrContains(allowedNonComServers, guild.ID, util.CaseSensitive) && oldStarterRole != nil {
		// message may have other bot related commands, but not with a prefix
		readRules := "I want to venture into the abyss"
		sanitizedMessage := util.MakeAlphaOnly(message.Content)
		if strings.HasPrefix(strings.ToUpper(sanitizedMessage), strings.ToUpper(readRules)) {
			if !util.StrContains(member.Roles, oldStarterRole.ID, util.CaseSensitive) {
				// bail out if the user doesn't have the first starter role. Preventing any duplicate role creations/DMs
				return
			}
			if starterRole == nil || oldStarterRole == nil {
				log.Println("ERROR! Unable to find roles for guild " + guild.Name)
				return
			}
			session.ChannelMessageSend(message.ChannelID, "Welcome "+message.Author.Mention()+"! We hope you enjoy your stay in our Discord server! Make sure to head over #announcements and #bot-stuff to see whats new and get your roles!")
			session.GuildMemberRoleAdd(guild.ID, member.User.ID, starterRole.ID)
			session.GuildMemberRoleRemove(guild.ID, member.User.ID, oldStarterRole.ID)
			log.Println("Updated user <" + member.User.Username + "> after reading the rules")
		}
	}
}

/**
Global handler that is called whenever moebot successfully connects to discord
*/
func ready(session *discordgo.Session, event *discordgo.Ready) {
	status := ComPrefix + " help"
	err := session.UpdateStatus(0, status)
	if err != nil {
		log.Println("Error setting moebot status", err)
	}
	log.Println("Set moebot's status to", status)
}

/**
Helper handler to check if the message provided is a command and if so, executes the command
*/
func runCommand(session *discordgo.Session, message *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member) {
	messageParts := strings.Split(message.Content, " ")
	if len(messageParts) <= 1 {
		// bad command, missing command after prefix
		return
	}
	commandKey := strings.ToUpper(messageParts[1])

	if command, commPresent := commandsMap[commandKey]; commPresent {
		params := messageParts[2:]
		if !checker.HasPermission(message.Author.ID, member.Roles, command.GetPermLevel()) {
			session.ChannelMessageSend(channel.ID, "Sorry, this command has a minimum permission of "+db.SprintPermission(command.GetPermLevel()))
			log.Println("!!PERMISSION VIOLATION!! Processing command: " + commandKey + " from user: {" + message.Author.String() + "}| With Params:{" + strings.Join(params, ",") + "}")
			return
		}
		log.Println("Processing command: " + commandKey + " from user: {" + message.Author.String() + "}| With Params:{" + strings.Join(params, ",") + "}")
		session.ChannelTyping(message.ChannelID)
		pack := commands.NewCommPackage(session, message, guild, member, channel, params)
		command.Execute(&pack)
	}
}
