package commands

import (
	"bytes"
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/camd67/moebot/moebot_bot/util"
	"github.com/camd67/moebot/moebot_bot/util/db"
)

type PinMoveCommand struct {
	ShouldLoadPins bool
	pinnedMessages util.SyncUIDByChannelMap
	ready          bool
}

func (pc *PinMoveCommand) Execute(pack *CommPackage) {
	if !pc.ready {
		pack.session.ChannelMessageSend(pack.channel.ID, "Sorry, the pin move feature is still loading.")
		return
	}
	commMap := ParseCommand(pack.params, []string{"-channel", "-dest", "-text", "-delete"})
	sourceChannelText, hasSource := commMap["-channel"]
	destChannelText, hasDest := commMap["-dest"]
	_, hasTextParam := commMap["-text"]
	_, hasDeleteParam := commMap["-delete"]

	if !hasSource {
		pack.session.ChannelMessageSend(pack.channel.ID, "You must specify a source and destination channel for this command.")
		return
	}

	if hasDest && sourceChannelText == destChannelText {
		pack.session.ChannelMessageSend(pack.channel.ID, "Please provide two different channels for pin moving.")
		return
	}

	// validate to make sure the two channels exist.
	// These can easily be refactored when we switch to session state
	var sourceChannel *discordgo.Channel
	var destChannel *discordgo.Channel
	sourceChannelUid, sourceChannelValid := util.ExtractChannelIdFromString(sourceChannelText)
	var destChannelUid string
	// default to true on dest channel in case we didn't provide one
	destChannelValid := true
	if hasDest {
		destChannelUid, destChannelValid = util.ExtractChannelIdFromString(destChannelText)
	}
	if !sourceChannelValid || !destChannelValid {
		pack.session.ChannelMessageSend(pack.channel.ID, "Please provide your channels in the `#channel-name` format")
		return
	}
	for _, c := range pack.guild.Channels {
		if c.ID == sourceChannelUid {
			sourceChannel = c
		}
		if hasDest && c.ID == destChannelUid {
			destChannel = c
		}
	}
	if sourceChannel == nil {
		pack.session.ChannelMessageSend(pack.channel.ID, "That source channel doesn't exist, please provide a valid source channel in the #channel-name format")
		return
	}
	if hasDest && destChannel == nil {
		pack.session.ChannelMessageSend(pack.channel.ID, "That destination channel doesn't exist, please provide a valid destination channel in the "+
			"#channel-name format")
		return
	}

	server, err := db.ServerQueryOrInsert(pack.guild.ID)
	if err != nil {
		pack.session.ChannelMessageSend(pack.channel.ID, "Sorry, there was an issue finding this server. This is an issue with moebot not Discord")
		return
	}

	dbChannel, err := db.ChannelQueryOrInsert(sourceChannel.ID, &server)
	if err != nil {
		pack.session.ChannelMessageSend(pack.channel.ID, "Sorry, there was an error getting the channel. This is an issue with moebot not Discord.")
		return
	}
	if !dbChannel.MoveChannelUid.Valid && !hasDest {
		pack.session.ChannelMessageSend(pack.channel.ID, "The provided channel doesn't have a destination. Please provide one.")
		return
	}

	// Overwrite with our new properties
	dbChannel.MovePins = true
	if hasDest {
		dbChannel.MoveChannelUid.Scan(destChannel.ID)
	}
	dbChannel.MoveTextPins = hasTextParam
	dbChannel.DeletePin = hasDeleteParam

	err = db.ChannelUpdate(dbChannel)
	if err != nil {
		pack.session.ChannelMessageSend(pack.channel.ID, "Sorry, there was an error updating the channel. This is an issue with moebot not Discord.")
		return
	}

	// Then load the pins if necessary
	pc.pinnedMessages.Lock()
	if _, pinsLoaded := pc.pinnedMessages.M[sourceChannel.ID]; !pinsLoaded {
		log.Println("Loading channel: " + sourceChannel.Name)
		go pc.loadChannel(pack.session, &server, sourceChannel)
	}
	pc.pinnedMessages.Unlock()

	var message bytes.Buffer
	message.WriteString("Message move on pin has been ")
	if dbChannel.MovePins {
		message.WriteString("enabled")
	} else {
		message.WriteString("disabled")
	}
	message.WriteString(" on channel <#")
	message.WriteString(sourceChannel.ID)
	message.WriteString(">. Sending pinned images to <#")
	message.WriteString(dbChannel.MoveChannelUid.String)
	message.WriteString(">")
	if dbChannel.MoveTextPins {
		message.WriteString(" Also moving text pins.")
	} else {
		message.WriteString(" Not including text pins.")
	}
	if dbChannel.DeletePin {
		message.WriteString(" Deleting any pinned messages when moved.")
	} else {
		message.WriteString(" Not deleting any pinned messages when moved.")
	}
	pack.session.ChannelMessageSend(pack.channel.ID, message.String())
}

func (pc *PinMoveCommand) Setup(session *discordgo.Session) {
	if pc.ShouldLoadPins {
		pc.pinnedMessages = util.SyncUIDByChannelMap{
			RWMutex: sync.RWMutex{},
			M:       make(map[string][]string),
		}
		guilds, err := session.UserGuilds(100, "", "")
		if err != nil {
			log.Println("Error loading guilds, some functions may not work correctly.", err)
			return
		}
		go pc.loadGuilds(session, guilds)
	} else {
		log.Println("!!! WARNING !!! Skipping loading pins. NOTE: this will break the ability to use the pin move command")
	}
}

func (pc *PinMoveCommand) EventHandlers() []interface{} {
	return []interface{}{pc.channelMovePinsUpdate}
}

func (pc *PinMoveCommand) loadGuilds(session *discordgo.Session, guilds []*discordgo.UserGuild) {
	for _, guild := range guilds {
		pc.loadGuild(session, guild)
	}
	pc.ready = true
}

func (pc *PinMoveCommand) loadGuild(session *discordgo.Session, guild *discordgo.UserGuild) {
	server, err := db.ServerQueryOrInsert(guild.ID)
	if err != nil {
		log.Println("Error creating/retrieving server during loading", err)
		return
	}
	channels, err := session.GuildChannels(guild.ID)
	if err != nil {
		log.Println("Error retrieving channels during loading", err)
		return
	}
	dbChannels, err := db.ChannelQueryByServer(server)
	for _, channel := range channels {
		//only loading text channels for now
		if channel.Type == discordgo.ChannelTypeGuildText {
			for _, dbC := range dbChannels {
				// also only load text channels which have pin moving enabled
				if dbC.ChannelUid == channel.ID && dbC.MovePins {
					log.Println("LOADING CHANNEL: " + channel.Name)
					pc.loadChannel(session, &server, channel)
				}
			}
			log.Println("Done processing channel: " + channel.Name)
		}
	}
}

func (pc *PinMoveCommand) loadChannel(session *discordgo.Session, server *db.Server, channel *discordgo.Channel) {
	_, err := db.ChannelQueryOrInsert(channel.ID, server)
	if err != nil {
		log.Println("Error creating/retrieving channel during loading", err)
		return
	}
	pc.loadPinnedMessages(session, channel)
}

func (pc *PinMoveCommand) loadPinnedMessages(session *discordgo.Session, channel *discordgo.Channel) {
	var pinnedMessages []string
	messages, err := session.ChannelMessagesPinned(channel.ID)
	if err != nil {
		log.Println("Error retrieving pinned channel messages", err)
	}
	for _, message := range messages {
		pinnedMessages = append(pinnedMessages, message.ID)
	}
	pc.pinnedMessages.Lock()
	pc.pinnedMessages.M[channel.ID] = pinnedMessages
	pc.pinnedMessages.Unlock()
}

func (pc *PinMoveCommand) channelMovePinsUpdate(session *discordgo.Session, pinsUpdate *discordgo.ChannelPinsUpdate) {
	if !pc.ready {
		log.Println("Pinmove is still loading, exiting pin handler")
		return
	}
	channel, err := session.Channel(pinsUpdate.ChannelID)
	if err != nil {
		log.Println("Error while retrieving channel by UID", err)
		return
	}
	server, err := db.ServerQueryOrInsert(channel.GuildID)
	if err != nil {
		log.Println("Error while retrieving server from database", err)
		return
	}
	dbChannel, err := db.ChannelQueryOrInsert(pinsUpdate.ChannelID, &server)
	if err != nil {
		log.Println("Error while retrieving source channel from database", err)
		return
	}
	if !dbChannel.MovePins || !dbChannel.MoveChannelUid.Valid {
		return
	}
	newPinnedMessages, err := pc.getUpdatePinnedMessages(session, pinsUpdate.ChannelID)
	if err != nil {
		log.Println("Error while retrieving new pinned messages", err)
		return
	}
	if len(newPinnedMessages) == 0 || len(newPinnedMessages) > 1 {
		return //removed pin or the bot is not in sync with the server, abort pinning operation
	}
	newPinnedMessage := newPinnedMessages[0]
	moveMessage := false
	for _, a := range newPinnedMessage.Attachments { //image from direct upload
		if strings.Contains(mime.TypeByExtension(filepath.Ext(a.Filename)), "image") {
			moveMessage = true
			break
		}
	}

	if !moveMessage && len(newPinnedMessage.Embeds) == 1 { //image from link
		if newPinnedMessage.Embeds[0].Type == "image" {
			moveMessage = true
		}
	}
	if len(newPinnedMessage.Attachments) == 0 && len(newPinnedMessage.Embeds) == 0 && dbChannel.MoveTextPins {
		moveMessage = true
	}
	if moveMessage {
		util.MoveMessage(session, newPinnedMessage, dbChannel.MoveChannelUid.String, dbChannel.DeletePin)
	}
}

func (pc *PinMoveCommand) getUpdatePinnedMessages(session *discordgo.Session, channelId string) (result []*discordgo.Message, err error) {
	currentPinnedMessages, err := session.ChannelMessagesPinned(channelId)
	var messagesId []string
	if err != nil {
		return
	}
	for _, m := range currentPinnedMessages {
		if !pc.pinnedMessageAlreadyLoaded(m.ID, channelId) {
			result = append(result, m)
		}
		messagesId = append(messagesId, m.ID)
	}
	pc.pinnedMessages.Lock()
	pc.pinnedMessages.M[channelId] = messagesId //refreshes pinned messages in case of messages removed from pins
	pc.pinnedMessages.Unlock()
	return
}

func (pc *PinMoveCommand) pinnedMessageAlreadyLoaded(messageId string, channelId string) bool {
	pc.pinnedMessages.RLock()
	defer pc.pinnedMessages.RUnlock()
	for _, m := range pc.pinnedMessages.M[channelId] {
		if messageId == m {
			return true
		}
	}
	return false
}

func (pc *PinMoveCommand) GetPermLevel() db.Permission {
	return db.PermMod
}

func (pc *PinMoveCommand) GetCommandKeys() []string {
	return []string{"PINMOVE"}
}

func (pc *PinMoveCommand) GetCommandHelp(commPrefix string) string {
	return fmt.Sprintf("`%[1]s pinmove -channel <#sourceChannel> -dest <#destChannel> [-text -delete]` - Enables moving pinned messages from one channel to "+
		"another. The `-dest` option sets/changes the destination channel. The `-text` option enables moving text as well as images on pin. The `-delete` "+
		"option will delete the message before moving.", commPrefix)
}
