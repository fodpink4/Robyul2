package plugins

import (
	"net/http"
	"strings"

	"strconv"

	"time"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/bradfitz/slice"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

type moveAction func(args []string, in *discordgo.Message, out **discordgo.MessageSend) (next moveAction)

type Move struct {
}

func (m *Move) Commands() []string {
	return []string{
		"move",
		"copy",
	}
}

func (m *Move) Init(session *discordgo.Session) {
}

func (m *Move) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	defer helpers.Recover()

	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermMod) {
		return
	}

	var result *discordgo.MessageSend
	args := strings.Fields(content)

	var action moveAction
	switch command {
	case "move":
		action = m.actionMove
	case "copy":
		action = m.actionCopy
	}

	for action != nil {
		action = action(args, msg, &result)
	}
}

// [p]move <#target channel or channel id> <message id> [<number of messages, min 1, max 100>]
// moves messages and deletes old messages after that
func (m *Move) actionMove(args []string, in *discordgo.Message, out **discordgo.MessageSend) moveAction {
	if !helpers.IsMod(in) {
		*out = m.newMsg("no_permission")
		return m.actionFinish
	}

	if len(args) < 2 {
		*out = m.newMsg("bot.arguments.too-few")
		return m.actionFinish
	}

	cache.GetSession().MessageReactionAdd(in.ChannelID, in.ID, "🔄")

	sourceChannel, err := helpers.GetChannel(in.ChannelID)
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	targetChannel, err := helpers.GetChannelFromMention(in, args[0])
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	// how many messages shall we move?
	numOfMessagesToMove := 1
	if len(args) >= 3 {
		numOfMessagesToMove, err = strconv.Atoi(args[2])
		if err != nil {
			cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
			*out = m.newMsg("bot.arguments.invalid")
			return m.actionFinish
		}
	}
	if numOfMessagesToMove < 1 || numOfMessagesToMove > 100 {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		*out = m.newMsg("bot.arguments.invalid")
		return m.actionFinish
	}

	err = m.copyMessages(sourceChannel.ID, args[1], numOfMessagesToMove, targetChannel.ID, true)
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
	cache.GetSession().MessageReactionAdd(in.ChannelID, in.ID, "👌")
	go func() {
		defer helpers.Recover()

		time.Sleep(5 * time.Second)
		cache.GetSession().ChannelMessageDelete(in.ChannelID, in.ID)
	}()
	return nil
}

// [p]copy <#target channel or channel id> <message id> [<number of messages, min 1, max 100>]
// moves messages and keeps the old messages
func (m *Move) actionCopy(args []string, in *discordgo.Message, out **discordgo.MessageSend) moveAction {
	if !helpers.IsMod(in) {
		*out = m.newMsg("no_permission")
		return m.actionFinish
	}

	if len(args) < 2 {
		*out = m.newMsg("bot.arguments.too-few")
		return m.actionFinish
	}

	cache.GetSession().MessageReactionAdd(in.ChannelID, in.ID, "🔄")

	sourceChannel, err := helpers.GetChannel(in.ChannelID)
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	targetChannel, err := helpers.GetChannelFromMention(in, args[0])
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	// how many messages shall we move?
	numOfMessagesToMove := 1
	if len(args) >= 3 {
		numOfMessagesToMove, err = strconv.Atoi(args[2])
		if err != nil {
			cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
			*out = m.newMsg("bot.arguments.invalid")
			return m.actionFinish
		}
	}
	if numOfMessagesToMove < 1 || numOfMessagesToMove > 100 {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		*out = m.newMsg("bot.arguments.invalid")
		return m.actionFinish
	}

	err = m.copyMessages(sourceChannel.ID, args[1], numOfMessagesToMove, targetChannel.ID, false)
	if err != nil {
		cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
		helpers.Relax(err)
	}

	cache.GetSession().MessageReactionRemove(in.ChannelID, in.ID, "🔄", cache.GetSession().State.User.ID)
	cache.GetSession().MessageReactionAdd(in.ChannelID, in.ID, "👌")
	go func() {
		defer helpers.Recover()

		time.Sleep(5 * time.Second)
		cache.GetSession().ChannelMessageDelete(in.ChannelID, in.ID)
	}()
	return nil
}

func (m *Move) copyMessages(sourceChannelID, sourceMessageID string, numberOfMessages int, targetChannelID string, delete bool) (err error) {
	// debug
	//m.logger().Debugf("requested move from #%s Message #%s n %d to #%s deletion %v",
	//	sourceChannelID, sourceMessageID, numberOfMessages, targetChannelID, delete)
	// gather messages to copy
	messagesToMove := make([]*discordgo.Message, 0)
	requestedMessage, err := cache.GetSession().ChannelMessage(sourceChannelID, sourceMessageID)
	if err != nil {
		return err
	}
	messagesToMove = append(messagesToMove, requestedMessage)
	// gather exact message
	// gather after messages
	if numberOfMessages > 1 {
		messagesLeft := numberOfMessages - 1
		lastAfterID := sourceMessageID
		for {
			messagesToGet := messagesLeft
			if messagesLeft > 100 {
				messagesToGet = 100
			}
			messagesLeft -= messagesToGet

			requestedMessages, err := cache.GetSession().ChannelMessages(sourceChannelID, messagesToGet, "", lastAfterID, "")
			if err != nil {
				return err
			}
			slice.Sort(requestedMessages, func(i, j int) bool {
				return requestedMessages[i].Timestamp < requestedMessages[j].Timestamp
			})
			for _, requestedMessage := range requestedMessages {
				messagesToMove = append(messagesToMove, requestedMessage)
				lastAfterID = requestedMessage.ID
			}

			if messagesLeft <= 0 {
				break
			}
		}
	}
	// get two webhooks for target channel (rotation)
	webhooksToUse := make([]*discordgo.Webhook, 0)
	existingWebhooks, err := cache.GetSession().ChannelWebhooks(targetChannelID)
	if err != nil {
		return err
	}
	if existingWebhooks != nil && len(existingWebhooks) > 0 {
		for _, existingWebhook := range existingWebhooks {
			webhooksToUse = append(webhooksToUse, existingWebhook)
			if len(webhooksToUse) >= 2 {
				break
			}
		}
	}
	for {
		if len(webhooksToUse) >= 2 {
			break
		}
		newWebhook, err := cache.GetSession().WebhookCreate(targetChannelID, "Robyul Webhook", "")
		if err != nil {
			return err
		}
		webhooksToUse = append(webhooksToUse, newWebhook)
	}
	// send new messages
	var lastUserID string
	var nextWebhookIndex int
	var nextContent string
	for _, messageToMove := range messagesToMove {
		nextContent = messageToMove.Content
		// should we switch to a new webhook (rotation)
		if lastUserID != messageToMove.Author.ID {
			nextWebhookIndex++
			if nextWebhookIndex > 1 {
				nextWebhookIndex = 0
			}
		}
		// gather file if attachments on message
		if messageToMove.Attachments != nil && len(messageToMove.Attachments) > 0 {
			data, err := helpers.NetGetUAWithError(messageToMove.Attachments[0].URL, helpers.DEFAULT_UA)
			helpers.RelaxLog(err)
			if err == nil {
				// sniff filetype from first 512 bytes
				contentType := http.DetectContentType(data[0:511])
				// debug
				//m.logger().Debugf("found attached file %s content type %s",
				//	messageToMove.Attachments[0].Filename, contentType)
				// reupload images to imgur
				if strings.HasPrefix(contentType, "image/") {
					newLink, err := helpers.UploadImage(data)
					helpers.RelaxLog(err)
					if err == nil {
						nextContent += "\n" + newLink
					}
				}
			}
		}
		// debug
		//m.logger().Debugf("posting webhook id %s by %s content %s",
		//	webhooksToUse[nextWebhookIndex].ID, messageToMove.Author.ID, messageToMove.Content,
		//)
		// send message
		if nextContent == "" && (messageToMove.Embeds == nil || len(messageToMove.Embeds) < 1) {
			continue
		}
		_, err := helpers.WebhookExecuteWithResult(
			webhooksToUse[nextWebhookIndex].ID,
			webhooksToUse[nextWebhookIndex].Token,
			&discordgo.WebhookParams{
				Content:   nextContent,
				Username:  messageToMove.Author.Username,
				AvatarURL: messageToMove.Author.AvatarURL("512"),
				TTS:       false,
				Embeds:    messageToMove.Embeds,
			},
		)
		if err != nil {
			return err
		}
		lastUserID = messageToMove.Author.ID
	}
	// delete messages if wanted
	if delete {
		bulkDeleteMessageIDs := make([]string, 0)
		for _, messageToDelete := range messagesToMove {
			bulkDeleteMessageIDs = append(bulkDeleteMessageIDs, messageToDelete.ID)
		}
		err := cache.GetSession().ChannelMessagesBulkDelete(sourceChannelID, bulkDeleteMessageIDs)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Move) actionFinish(args []string, in *discordgo.Message, out **discordgo.MessageSend) moveAction {
	_, err := helpers.SendComplex(in.ChannelID, *out)
	helpers.Relax(err)

	return nil
}

func (m *Move) newMsg(content string) *discordgo.MessageSend {
	return &discordgo.MessageSend{Content: helpers.GetText(content)}
}

func (m *Move) Relax(err error) {
	if err != nil {
		panic(err)
	}
}

func (m *Move) logger() *logrus.Entry {
	return cache.GetLogger().WithField("module", "move")
}
