package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/models"
	"github.com/bradfitz/slice"
	"github.com/bwmarrin/discordgo"
	"github.com/getsentry/raven-go"
	"github.com/globalsign/mgo/bson"
	redisCache "github.com/go-redis/cache"
	"github.com/vmihailenco/msgpack"
)

const (
	DISCORD_EPOCH                     int64 = 1420070400000
	DISCORD_DARK_THEME_BACKGROUND_HEX       = "#36393F"
)

var (
	snowflakeRegex     = regexp.MustCompile(`^[0-9]+$`)
	discordInviteRegex = regexp.MustCompile(`(http(s)?:\/\/)?(discord\.gg(\/invite)?|discordapp\.com\/invite)\/([A-Za-z0-9]+)`)
)

var botAdmins = []string{
	"116620585638821891", // Sekl
	"134298438559858688", // Kakkela
}
var NukeMods = []string{
	"116620585638821891", // Sekl
	"134298438559858688", // Kakkela
	"68661361537712128",  // Berk
}
var RobyulMod = []string{
	"273639623324991489", // Snakeyez
}
var Blacklisted = []string{
	"171883318386753536", // ForRyu
}
var BlacklistedGuildIDs []string

// No Level gaining, No Elastic Search features
var LimitedGuildIDs = []string{
	"264445053596991498", // Discord Bot List
}
var BasicInspectRoleIDs = []string{
	"345209098100277248", // :hammer: (.kpop)
}
var ExtendedInspectRoleIDs = []string{
	"345209385821274113", // inspect extended (sekl's dev cord)
	"345209098100277248", // inspect (Moderator Chat)
}
var adminRoleNames = []string{"Admin", "Admins", "ADMIN", "School Board", "admin", "admins"}
var modRoleNames = []string{"Mod", "Mods", "Mod Trainee", "Moderator", "Moderators", "MOD", "Minimod", "Guard", "Janitor", "mod", "mods", "Budget Admin"}

func IsBlacklisted(id string) bool {
	for _, s := range Blacklisted {
		if s == id {
			return true
		}
	}

	return false
}

func IsBlacklistedGuild(guildID string) bool {
	for _, s := range BlacklistedGuildIDs {
		if s == guildID {
			return true
		}
	}

	return false
}

func IsLimitedGuild(guildID string) bool {
	for _, s := range LimitedGuildIDs {
		if s == guildID {
			return true
		}
	}

	return false
}

func IsNukeMod(id string) bool {
	for _, s := range NukeMods {
		if s == id {
			return true
		}
	}

	return false
}

// IsBotAdmin checks if $id is in $botAdmins
func IsBotAdmin(id string) bool {
	for _, s := range botAdmins {
		if s == id {
			return true
		}
	}

	return false
}

func IsRobyulMod(id string) bool {
	if IsBotAdmin(id) {
		return true
	}
	for _, s := range RobyulMod {
		if s == id {
			return true
		}
	}

	return false
}

func CanInspectBasic(msg *discordgo.Message) bool {
	if CanInspectExtended(msg) {
		return true
	}

	channel, e := GetChannel(msg.ChannelID)
	if e != nil {
		return false
	}

	guild, e := GetGuild(channel.GuildID)
	if e != nil {
		return false
	}

	guildMember, e := GetGuildMemberWithoutApi(guild.ID, msg.Author.ID)
	if e != nil {
		return false
	}
	for _, role := range guild.Roles {
		for _, userRole := range guildMember.Roles {
			if userRole == role.ID {
				for _, inspectRoleID := range BasicInspectRoleIDs {
					if role.ID == inspectRoleID {
						return true
					}
				}
			}
		}
	}
	return false
}

func CanInspectExtended(msg *discordgo.Message) bool {
	if IsBotAdmin(msg.Author.ID) {
		return true
	}

	if IsRobyulMod(msg.Author.ID) {
		return true
	}

	if IsNukeMod(msg.Author.ID) {
		return true
	}

	channel, e := GetChannel(msg.ChannelID)
	if e != nil {
		return false
	}

	guild, e := GetGuild(channel.GuildID)
	if e != nil {
		return false
	}

	guildMember, e := GetGuildMemberWithoutApi(guild.ID, msg.Author.ID)
	if e != nil {
		return false
	}
	for _, role := range guild.Roles {
		for _, userRole := range guildMember.Roles {
			if userRole == role.ID {
				for _, inspectRoleID := range ExtendedInspectRoleIDs {
					if role.ID == inspectRoleID {
						return true
					}
				}
			}
		}
	}
	return false
}

func IsAdmin(msg *discordgo.Message) bool {
	channel, err := GetChannel(msg.ChannelID)
	if err != nil {
		return false
	}

	return IsAdminByID(channel.GuildID, msg.Author.ID)
}

func IsAdminByID(guildID string, userID string) bool {
	guild, err := GetGuild(guildID)
	if err != nil {
		return false
	}

	if userID == guild.OwnerID || IsBotAdmin(userID) {
		return true
	}

	guildMember, err := GetGuildMemberWithoutApi(guild.ID, userID)
	if err != nil {
		return false
	}

	adminRoleIDs := GuildSettingsGetCached(guildID).AdminRoleIDs

	for _, role := range guild.Roles {
		for _, userRole := range guildMember.Roles {
			if userRole == role.ID {
				// Check if role may manage server or a role is in admin role list
				if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
					return true
				}
				if adminRoleIDs == nil || len(adminRoleIDs) <= 0 {
					// role name matching if no custom role has been set
					for _, adminRoleName := range adminRoleNames {
						if role.Name == adminRoleName {
							return true
						}
					}
				} else {
					// check for custom role if has been set
					for _, adminRoleID := range adminRoleIDs {
						if role.ID == adminRoleID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func HasPermissionByID(guildID string, userID string, permission int) bool {
	guild, e := GetGuild(guildID)
	if e != nil {
		return false
	}

	if userID == guild.OwnerID {
		return true
	}

	guildMember, e := GetGuildMemberWithoutApi(guild.ID, userID)
	if e != nil {
		return false
	}
	for _, role := range guild.Roles {
		for _, userRole := range guildMember.Roles {
			if userRole == role.ID {
				if role.Permissions&permission == permission {
					return true
				}
			}
		}
	}
	return false
}

func IsMod(msg *discordgo.Message) bool {
	channel, err := GetChannel(msg.ChannelID)
	if err != nil {
		return false
	}

	return IsModByID(channel.GuildID, msg.Author.ID)
}

func IsModByID(guildID string, userID string) bool {
	if IsAdminByID(guildID, userID) {
		return true
	} else {
		guild, e := GetGuild(guildID)
		if e != nil {
			return false
		}
		guildMember, e := GetGuildMemberWithoutApi(guild.ID, userID)
		if e != nil {
			return false
		}

		modRoleIDs := GuildSettingsGetCached(guildID).ModRoleIDs

		// check if a role is in mod role list
		for _, role := range guild.Roles {
			for _, userRole := range guildMember.Roles {
				if userRole == role.ID {
					if modRoleIDs == nil || len(modRoleIDs) <= 0 {
						// role name matching if no custom role has been set
						for _, modRoleName := range modRoleNames {
							if role.Name == modRoleName {
								return true
							}
						}
					} else {
						// check for custom role if has been set
						for _, modRoleID := range modRoleIDs {
							if role.ID == modRoleID {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// RequireAdmin only calls $cb if the author is an admin or has MANAGE_SERVER permission
func RequireAdmin(msg *discordgo.Message, cb Callback) {
	if !IsAdmin(msg) {
		SendMessage(msg.ChannelID, GetText("admin.no_permission"))
		return
	}

	cb()
}

// RequireAdmin only calls $cb if the author is an admin or has MANAGE_SERVER permission
func RequireAdminOrStaff(msg *discordgo.Message, cb Callback) {
	if !IsAdmin(msg) && !IsRobyulMod(msg.Author.ID) {
		SendMessage(msg.ChannelID, GetText("admin.no_permission"))
		return
	}

	cb()
}

// RequireAdmin only calls $cb if the author is an admin or has MANAGE_SERVER permission
func RequireMod(msg *discordgo.Message, cb Callback) {
	if !IsMod(msg) {
		SendMessage(msg.ChannelID, GetText("mod.no_permission"))
		return
	}

	cb()
}

// RequireBotAdmin only calls $cb if the author is a bot admin
func RequireBotAdmin(msg *discordgo.Message, cb Callback) {
	if !IsBotAdmin(msg.Author.ID) {
		SendMessage(msg.ChannelID, GetText("botadmin.no_permission"))
		return
	}

	cb()
}

// RequireSupportMod only calls $cb if the author is a support mod
func RequireRobyulMod(msg *discordgo.Message, cb Callback) {
	if !IsRobyulMod(msg.Author.ID) {
		SendMessage(msg.ChannelID, GetText("robyulmod.no_permission"))
		return
	}

	cb()
}

func ConfirmEmbed(channelID string, author *discordgo.User, confirmMessageText string, confirmEmojiID string, abortEmojiID string) bool {
	// send embed asking the user to confirm
	confirmMessages, err := SendComplex(channelID,
		&discordgo.MessageSend{
			Content: "<@" + author.ID + ">",
			Embed: &discordgo.MessageEmbed{
				Title:       GetText("bot.embeds.please-confirm-title"),
				Description: confirmMessageText,
			},
		})
	if err != nil {
		SendMessage(channelID, GetTextF("bot.errors.general", err.Error()))
		return false
	}
	if len(confirmMessages) <= 0 {
		SendMessage(channelID, GetText("bot.errors.generic-nomessage"))
		return false
	}
	confirmMessage := confirmMessages[0]
	if len(confirmMessage.Embeds) <= 0 {
		SendMessage(channelID, GetText("bot.errors.no-embed"))
		return false
	}

	// add default reactions to embed
	cache.GetSession().MessageReactionAdd(confirmMessage.ChannelID, confirmMessage.ID, confirmEmojiID)
	cache.GetSession().MessageReactionAdd(confirmMessage.ChannelID, confirmMessage.ID, abortEmojiID)

	responseChannel := make(chan bool, 1)
	stopHandler := cache.GetSession().AddHandler(func(session *discordgo.Session, reaction *discordgo.MessageReactionAdd) {
		if reaction == nil || reaction.MessageID != confirmMessage.ID || reaction.UserID != author.ID {
			return
		}

		if reaction.Emoji.Name == confirmEmojiID { // has confirmed
			responseChannel <- true
		}
		if reaction.Emoji.Name == abortEmojiID { // has denied
			responseChannel <- false
		}
	})

	go func() {
		time.Sleep(3 * time.Hour)
		responseChannel <- false
	}()

	response := <-responseChannel
	stopHandler()
	cache.GetSession().ChannelMessageDelete(confirmMessage.ChannelID, confirmMessage.ID)
	return response
}

func GetMuteRole(guildID string) (*discordgo.Role, error) {
	guild, err := GetGuild(guildID)
	Relax(err)
	var muteRole *discordgo.Role
	settings, err := GuildSettingsGet(guildID)
	for _, role := range guild.Roles {
		Relax(err)
		if role.Name == settings.MutedRoleName {
			muteRole = role
		}
	}
	if muteRole == nil {
		muteRole, err = cache.GetSession().GuildRoleCreate(guildID)
		if err != nil {
			return muteRole, err
		}
		muteRole, err = cache.GetSession().GuildRoleEdit(guildID, muteRole.ID, settings.MutedRoleName, muteRole.Color, muteRole.Hoist, 0, muteRole.Mentionable)
		if err != nil {
			return muteRole, err
		}
		for _, channel := range guild.Channels {
			err = cache.GetSession().ChannelPermissionSet(channel.ID, muteRole.ID, "role", 0,
				discordgo.PermissionSendMessages|discordgo.PermissionAddReactions|discordgo.PermissionVoiceConnect)
			if err != nil {
				cache.GetLogger().WithField("module", "discord").Error("Error disabling send messages and add reactions on mute Role: " + err.Error())
			}
		}
	}
	return muteRole, nil
}

func RemoveMuteRole(guildID string, userID string) (err error) {
	muteRole, err := GetMuteRole(guildID)
	if err != nil {
		return err
	}

	err = cache.GetSession().GuildMemberRoleRemove(guildID, userID, muteRole.ID)
	if err != nil {
		if errD, ok := err.(*discordgo.RESTError); ok && errD.Message != nil {
			if errD.Message.Code == discordgo.ErrCodeUnknownMember ||
				errD.Message.Code == discordgo.ErrCodeUnknownUser ||
				errD.Response.StatusCode == 404 {
				return nil
			}
		}
	}
	return err
}

func RemoveMuteDatabase(guildID string, userID string) (err error) {
	settings := GuildSettingsGetCached(guildID)

	removedFromDb := false
	newMutedMembers := make([]string, 0)
	for _, mutedMember := range settings.MutedMembers {
		if mutedMember != userID {
			newMutedMembers = append(newMutedMembers, mutedMember)
		} else {
			removedFromDb = true
		}
	}

	if removedFromDb {
		settings.MutedMembers = newMutedMembers
		err = GuildSettingsSet(guildID, settings)
		return err
	}
	return nil
}

func RemoveMutePersistency(guildID string, userID string) (err error) {
	muteRole, err := GetMuteRole(guildID)
	if err != nil {
		return err
	}

	return persistencyRemoveCachedRole(guildID, userID, muteRole.ID)
}

func RemovePendingUnmutes(guildID string, userID string) (err error) {
	key := "delayed_tasks"
	delayedTasks, err := cache.GetMachineryRedisClient().ZCard(key).Result()
	if err != nil {
		return err
	}

	tasksJson, err := cache.GetMachineryRedisClient().ZRange(key, 0, delayedTasks).Result()
	if err != nil {
		return err
	}

	for _, taskJson := range tasksJson {
		task, err := gabs.ParseJSON([]byte(taskJson))
		if err != nil {
			return err
		}

		if task.Path("Name").Data().(string) != "unmute_user" {
			continue
		}

		unmuteGuildID := task.Path("Args").Index(0).Path("Value").Data().(string)
		unmuteUserID := task.Path("Args").Index(1).Path("Value").Data().(string)

		if unmuteGuildID != guildID {
			continue
		}
		if unmuteUserID != userID {
			continue
		}

		_, err = cache.GetMachineryRedisClient().ZRem(key, taskJson).Result()
		if err != nil {
			return err
		}
	}

	return nil
}

func UnmuteUserMachinery(guildID string, userID string) (err error) {
	err = UnmuteUser(guildID, userID)

	if err == nil {
		_, err = EventlogLog(time.Now(), guildID, userID,
			models.EventlogTargetTypeUser, cache.GetSession().State.User.ID,
			models.EventlogTypeRobyulUnmute, "timed mute expired",
			nil,
			nil, false)
		RelaxLog(err)
	}

	return err
}

func UnmuteUser(guildID string, userID string) (err error) {
	errRole := RemoveMuteRole(guildID, userID)
	errDatabase := RemoveMuteDatabase(guildID, userID)
	errPersistency := RemoveMutePersistency(guildID, userID)
	errPendingUnmutes := RemovePendingUnmutes(guildID, userID)

	if errRole != nil {
		return errRole
	}
	if errDatabase != nil {
		return errDatabase
	}
	if errPersistency != nil {
		return errPersistency
	}
	if errPendingUnmutes != nil {
		return errPendingUnmutes
	}
	return nil
}
func UnmuteUserSignature(guildID string, userID string) (signature *tasks.Signature) {
	signature = &tasks.Signature{
		Name: "unmute_user",
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: guildID,
			},
			{
				Type:  "string",
				Value: userID,
			},
		},
	}
	signature.RetryCount = 3
	signature.OnError = []*tasks.Signature{{Name: "log_error"}}
	return signature
}

func AddMuteRole(guildID string, userID string) (err error) {
	muteRole, err := GetMuteRole(guildID)
	if err != nil {
		return err
	}

	if GetIsInGuild(guildID, userID) {
		err = cache.GetSession().GuildMemberRoleAdd(guildID, userID, muteRole.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func AddMutePersistency(guildID, userID string) (err error) {
	muteRole, err := GetMuteRole(guildID)
	if err != nil {
		return err
	}

	err = persistencyAddCachedRole(guildID, userID, muteRole.ID)
	if err != nil {
		return err
	}

	return nil
}

func CreatePendingUnmute(guildID string, userID string, unmuteAt time.Time) (err error) {
	if unmuteAt.IsZero() || !time.Now().Before(unmuteAt) {
		return nil
	}

	timeToUnmuteAt := unmuteAt

	signature := UnmuteUserSignature(guildID, userID)
	signature.ETA = &timeToUnmuteAt

	_, err = cache.GetMachineryServer().SendTask(signature)
	if err != nil {
		return err
	}

	return nil
}

func MuteUser(guildID string, userID string, unmuteAt time.Time) (err error) {
	errRole := AddMuteRole(guildID, userID)
	errAddMutePersistency := AddMutePersistency(guildID, userID)
	errPendingUnmutes := RemovePendingUnmutes(guildID, userID)
	errCreatePendingUnmute := CreatePendingUnmute(guildID, userID, unmuteAt)

	if errRole != nil {
		return errRole
	}
	if errAddMutePersistency != nil {
		return errAddMutePersistency
	}
	if errPendingUnmutes != nil {
		return errPendingUnmutes
	}
	if errCreatePendingUnmute != nil {
		return errCreatePendingUnmute
	}
	return nil
}

func persistencyAddCachedRole(GuildID string, UserID string, roleID string) (err error) {
	key := "robyul2-discord:persistency:" + GuildID + ":" + UserID + ":roles"
	var redisRoleIDs []string
	var dbRoles models.PersistencyRolesEntry

	// add to db
	MdbOne(
		MdbCollection(models.PersistencyRolesTable).Find(bson.M{"guildid": GuildID, "userid": UserID}),
		&dbRoles,
	)
	alreadyInDbRoles := false
	for _, dbRoleID := range dbRoles.Roles {
		if dbRoleID == roleID {
			alreadyInDbRoles = true
		}
	}

	if !alreadyInDbRoles {
		dbRoles.Roles = append(dbRoles.Roles, roleID)
	}

	if dbRoles.ID.Valid() {
		// update
		err = MDbUpdate(models.PersistencyRolesTable, dbRoles.ID, dbRoles)
		if err != nil {
			return err
		}
	} else {
		// insert
		dbRoles.GuildID = GuildID
		dbRoles.UserID = UserID

		_, err = MDbInsert(models.PersistencyRolesTable, dbRoles)
		if err != nil {
			return err
		}
	}

	// add to redis
	marshalled, err := cache.GetRedisClient().Get(key).Bytes()
	if err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return nil
		}
		return err
	}

	err = msgpack.Unmarshal(marshalled, &redisRoleIDs)
	if err != nil {
		return err
	}

	alreadyInRedisRoles := false
	for _, redisRoleID := range redisRoleIDs {
		if redisRoleID == roleID {
			alreadyInRedisRoles = true
		}
	}

	if !alreadyInRedisRoles {
		redisRoleIDs = append(redisRoleIDs, roleID)
	}

	marshalled, err = msgpack.Marshal(redisRoleIDs)
	if err != nil {
		return
	}

	err = cache.GetRedisClient().Set(key, marshalled, 0).Err()

	return err
}

func persistencyRemoveCachedRole(GuildID string, UserID string, roleID string) (err error) {
	key := "robyul2-discord:persistency:" + GuildID + ":" + UserID + ":roles"
	var redisRoleIDs []string
	var dbRoles models.PersistencyRolesEntry

	// remove from db
	MdbOne(
		MdbCollection(models.PersistencyRolesTable).Find(bson.M{"guildid": GuildID, "userid": UserID}),
		&dbRoles,
	)

	newDbRoles := dbRoles
	newDbRoles.Roles = make([]string, 0)
	for _, dbRoleID := range dbRoles.Roles {
		if dbRoleID != roleID {
			newDbRoles.Roles = append(newDbRoles.Roles, dbRoleID)
		}
	}

	if dbRoles.ID.Valid() {
		err = MDbDelete(models.PersistencyRolesTable, dbRoles.ID)
		if err != nil {
			return err
		}

	}

	// remove from redis
	marshalled, err := cache.GetRedisClient().Get(key).Bytes()
	if err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return nil
		}
		return err
	}

	err = msgpack.Unmarshal(marshalled, &redisRoleIDs)
	if err != nil {
		return err
	}

	newRedisRoleIDs := make([]string, 0)
	for _, redisRoleID := range redisRoleIDs {
		if redisRoleID != roleID {
			newRedisRoleIDs = append(newRedisRoleIDs, redisRoleID)
		}
	}

	marshalled, err = msgpack.Marshal(newRedisRoleIDs)
	if err != nil {
		return
	}

	err = cache.GetRedisClient().Set(key, marshalled, 0).Err()

	return err
}

func LogMachineryError(errorMessage string) (err error) {
	cache.GetLogger().WithField("module", "machinery").Error("Task Failed: ", errorMessage)
	return err
}

func GetGuildMember(guildID string, userID string) (*discordgo.Member, error) {
	targetMember, err := cache.GetSession().State.Member(guildID, userID)
	if targetMember == nil || targetMember.GuildID == "" || targetMember.JoinedAt == "" {
		cache.GetLogger().WithField("module", "discord").WithField("method", "GetGuildMember").Debug(
			fmt.Sprintf("discord api request: GuildMember: %s, %s", guildID, userID))
		targetMember, err = cache.GetSession().GuildMember(guildID, userID)
	}
	if targetMember != nil {
		targetMember.GuildID = guildID
	}
	return targetMember, err
}

func GetGuildMemberWithoutApi(guildID string, userID string) (*discordgo.Member, error) {
	return cache.GetSession().State.Member(guildID, userID)
}

func GetIsInGuild(guildID string, userID string) bool {
	member, err := GetGuildMemberWithoutApi(guildID, userID)
	if err == nil && member != nil && member.User != nil && member.User.ID != "" {
		return true
	} else {
		return false
	}
}

func GetGuild(guildID string) (*discordgo.Guild, error) {
	targetGuild, err := cache.GetSession().State.Guild(guildID)
	if targetGuild == nil || targetGuild.ID == "" {
		//cache.GetLogger().WithField("module", "discord").WithField("method", "GetGuild").Debug(
		//		fmt.Sprintf("discord api request: Guild: %s", guildID))
		targetGuild, err = cache.GetSession().Guild(guildID)
	}
	return targetGuild, err
}

func GetGuildWithoutApi(guildID string) (*discordgo.Guild, error) {
	targetGuild, err := cache.GetSession().State.Guild(guildID)
	return targetGuild, err
}

func GetChannel(channelID string) (*discordgo.Channel, error) {
	targetChannel, err := cache.GetSession().State.Channel(channelID)
	if targetChannel == nil || targetChannel.ID == "" {
		//cache.GetLogger().WithField("module", "discord").WithField("method", "GetChannel").Debug(
		//	fmt.Sprintf("discord api request: Channel: %s", channelID))
		targetChannel, err = cache.GetSession().Channel(channelID)
	}
	return targetChannel, err
}

func GetChannelWithoutApi(channelID string) (*discordgo.Channel, error) {
	targetChannel, err := cache.GetSession().State.Channel(channelID)
	return targetChannel, err
}

func GetMessage(channelID string, messageID string) (*discordgo.Message, error) {
	targetMessage, err := cache.GetSession().State.Message(channelID, messageID)
	if targetMessage == nil || targetMessage.ID == "" {
		//cache.GetLogger().WithField("module", "discord").WithField("method", "GetMessage").Debug(
		//	fmt.Sprintf("discord api request: Message: %s in Channel: %s", messageID, channelID))
		targetMessage, err = cache.GetSession().ChannelMessage(channelID, messageID)
		cache.GetSession().State.MessageAdd(targetMessage)
		return targetMessage, err
	}
	return targetMessage, nil
}

func GetChannelFromMention(msg *discordgo.Message, mention string) (*discordgo.Channel, error) {
	result, err := GetChannelOfAnyTypeFromMention(msg, mention)
	if err != nil {
		return nil, err
	}
	if result.Type != discordgo.ChannelTypeGuildText {
		return nil, errors.New("not a text channel")
	}
	return result, nil
}

func GetChannelOrCategoryFromMention(msg *discordgo.Message, mention string) (*discordgo.Channel, error) {
	result, err := GetChannelOfAnyTypeFromMention(msg, mention)
	if err != nil {
		return nil, err
	}
	if result.Type != discordgo.ChannelTypeGuildText && result.Type != discordgo.ChannelTypeGuildCategory {
		return nil, errors.New("not a text channel nor a category channel")
	}
	return result, nil
}

func GetChannelOfAnyTypeFromMention(msg *discordgo.Message, mention string) (*discordgo.Channel, error) {
	var targetChannel *discordgo.Channel
	re := regexp.MustCompile("(<#)?(\\d+)(>)?")
	result := re.FindStringSubmatch(mention)
	if len(result) == 4 {
		sourceChannel, err := GetChannel(msg.ChannelID)
		if err != nil {
			return targetChannel, err
		}
		if sourceChannel == nil {
			return targetChannel, errors.New("Channel not found.")
		}
		targetChannel, err := GetChannel(result[2])
		if err != nil {
			return targetChannel, err
		}
		if sourceChannel.GuildID != targetChannel.GuildID {
			return targetChannel, errors.New("Channel on different guild.")
		}
		return targetChannel, err
	} else {
		return targetChannel, errors.New("Channel not found.")
	}
}

func GetGlobalChannelFromMention(mention string) (*discordgo.Channel, error) {
	var targetChannel *discordgo.Channel
	re := regexp.MustCompile("(<#)?(\\d+)(>)?")
	result := re.FindStringSubmatch(mention)
	if len(result) == 4 {
		targetChannel, err := GetChannel(result[2])
		if err != nil {
			return targetChannel, err
		}
		return targetChannel, err
	} else {
		return targetChannel, errors.New("Channel not found.")
	}
}

func GetUser(userID string) (*discordgo.User, error) {
	var err error
	var targetUser discordgo.User
	cacheCodec := cache.GetRedisCacheCodec()
	key := fmt.Sprintf("robyul2-discord:api:user:%s", userID) // TODO: Should we cache this?

	for _, guild := range cache.GetSession().State.Guilds {
		member, err := GetGuildMemberWithoutApi(guild.ID, userID)
		if err == nil && member != nil && member.User != nil && member.User.ID != "" {
			return member.User, nil
		}
	}

	if err = cacheCodec.Get(key, &targetUser); err != nil {
		cache.GetLogger().WithField("module", "discord").WithField("method", "GetUser").Debug(
			fmt.Sprintf("discord api request: User: %s", userID))
		targetUser, err := cache.GetSession().User(userID)
		if err == nil {
			err = cacheCodec.Set(&redisCache.Item{
				Key:        key,
				Object:     targetUser,
				Expiration: time.Minute * 10,
			})
			if err != nil {
				raven.CaptureError(fmt.Errorf("%#v", err), map[string]string{})
			}
		}
		return targetUser, err
	}
	return &targetUser, err
}

func GetUserWithoutAPI(userID string) (*discordgo.User, error) {
	for _, guild := range cache.GetSession().State.Guilds {
		member, err := GetGuildMemberWithoutApi(guild.ID, userID)
		if err == nil && member != nil && member.User != nil && member.User.ID != "" {
			return member.User, nil
		}
	}
	return nil, errors.New("user not found")
}

func GetUserFromMention(mention string) (*discordgo.User, error) {
	re := regexp.MustCompile("(<@)?(\\d+)(>)?")
	result := re.FindStringSubmatch(mention)
	if len(result) == 4 {
		return GetUser(result[2])
	} else {
		return &discordgo.User{}, errors.New("user not found")
	}
}

func GetDiscordColorFromHex(hex string) int {
	colorInt, ok := new(big.Int).SetString(strings.Replace(hex, "#", "", 1), 16)
	if ok == true {
		return int(colorInt.Int64())
	} else {
		return 0x0FADED
	}
}

func GetHexFromDiscordColor(colour int) (hex string) {
	return strings.ToUpper(big.NewInt(int64(colour)).Text(16))
}

func GetTimeFromSnowflake(id string) time.Time {
	iid, err := strconv.ParseInt(id, 10, 64)
	Relax(err)

	return time.Unix(((iid>>22)+DISCORD_EPOCH)/1000, 0).UTC()
}

func GetAllPermissions(guild *discordgo.Guild, member *discordgo.Member) int64 {
	var perms int64 = 0
	for _, x := range guild.Roles {
		if x.Name == "@everyone" {
			perms |= int64(x.Permissions)
		}
	}
	for _, r := range member.Roles {
		for _, x := range guild.Roles {
			if x.ID == r {
				perms |= int64(x.Permissions)
			}
		}
	}
	return perms
}

func Pagify(text string, delimiter string) []string {
	result := make([]string, 0)
	textParts := strings.Split(text, delimiter)
	currentOutputPart := ""
	for _, textPart := range textParts {
		if len(currentOutputPart)+len(textPart)+len(delimiter) <= 1992 {
			if len(currentOutputPart) > 0 || len(result) > 0 {
				currentOutputPart += delimiter + textPart
			} else {
				currentOutputPart += textPart
			}
		} else {
			result = append(result, currentOutputPart)
			currentOutputPart = ""
			if len(textPart) <= 1992 { // @TODO: else: split text somehow
				currentOutputPart = textPart
			}
		}
	}
	if currentOutputPart != "" {
		result = append(result, currentOutputPart)
	}
	return result
}

func GetAvatarUrl(user *discordgo.User) string {
	return GetAvatarUrlWithSize(user, 1024)
}

func GetAvatarUrlWithSize(user *discordgo.User, size uint16) string {
	if user.Avatar == "" {
		return ""
	}

	avatarUrl := "https://cdn.discordapp.com/avatars/%s/%s.%s?size=%d"

	if strings.HasPrefix(user.Avatar, "a_") {
		return fmt.Sprintf(avatarUrl, user.ID, user.Avatar, "gif", size)
	}

	return fmt.Sprintf(avatarUrl, user.ID, user.Avatar, "jpg", size)
}

func CommandExists(name string) bool {
	for _, command := range cache.GetPluginList() {
		if strings.ToLower(command) == strings.ToLower(name) {
			return true
		}
	}
	for _, command := range cache.GetPluginExtendedList() {
		if strings.ToLower(command) == strings.ToLower(name) {
			return true
		}
	}
	return false
}

func GuildIsOnWhitelist(GuildID string) (whitelisted bool) {
	var entryBucket []models.AutoleaverWhitelistEntry
	err := MDbIter(MdbCollection(models.AutoleaverWhitelistTable).Find(nil)).All(&entryBucket)
	if err != nil {
		return false
	}

	for _, whitelistEntry := range entryBucket {
		if whitelistEntry.GuildID == GuildID {
			return true
		}
	}

	return false
}

func AutoPagify(text string) (pages []string) {
	for _, page := range Pagify(text, "\n") {
		if len(page) <= 1992 {
			pages = append(pages, page)
		} else {
			for _, page := range Pagify(page, ",") {
				if len(page) <= 1992 {
					pages = append(pages, page)
				} else {
					for _, page := range Pagify(page, "-") {
						if len(page) <= 1992 {
							pages = append(pages, page)
						} else {
							for _, page := range Pagify(page, " ") {
								if len(page) <= 1992 {
									pages = append(pages, page)
								} else {
									panic("unable to pagify text")
								}
							}
						}
					}
				}
			}
		}
	}
	return pages
}

func SendMessage(channelID, content string) (messages []*discordgo.Message, err error) {
	var message *discordgo.Message
	content = CleanDiscordContent(content)
	if len(content) > 2000 {
		for _, page := range AutoPagify(content) {
			message, err = cache.GetSession().ChannelMessageSend(channelID, page)
			if err != nil {
				return messages, err
			}
			messages = append(messages, message)
		}
	} else {
		message, err = cache.GetSession().ChannelMessageSend(channelID, content)
		if err != nil {
			return messages, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func SendMessageBoxed(channelID, content string) (messages []*discordgo.Message, err error) {
	var newMessages []*discordgo.Message
	content = CleanDiscordContent(content)
	for _, page := range AutoPagify(content) {
		newMessages, err = SendMessage(channelID, "```"+page+"```")
		if err != nil {
			return messages, err
		}
		messages = append(messages, newMessages...)
	}
	return messages, nil
}

func SendEmbed(channelID string, embed *discordgo.MessageEmbed) (messages []*discordgo.Message, err error) {
	var message *discordgo.Message
	message, err = cache.GetSession().ChannelMessageSendEmbed(channelID, TruncateEmbed(embed))
	if err != nil {
		return messages, err
	}
	messages = append(messages, message)
	return messages, nil
}

func SendFile(channelID string, filename string, reader io.Reader, message string) (messages []*discordgo.Message, err error) {
	return SendComplex(channelID, &discordgo.MessageSend{File: &discordgo.File{Name: filename, Reader: reader}, Content: message})
}

func SendComplex(channelID string, data *discordgo.MessageSend) (messages []*discordgo.Message, err error) {
	var message *discordgo.Message
	if data.Embed != nil {
		data.Embed = TruncateEmbed(data.Embed)
	}
	data.Content = CleanDiscordContent(data.Content)
	pages := AutoPagify(data.Content)
	if len(pages) > 0 {
		for i, page := range pages {
			if i+1 < len(pages) {
				message, err = cache.GetSession().ChannelMessageSend(channelID, page)
			} else {
				data.Content = page
				message, err = cache.GetSession().ChannelMessageSendComplex(channelID, data)
			}
			if err != nil {
				return messages, err
			}
			messages = append(messages, message)
		}
	} else {
		message, err = cache.GetSession().ChannelMessageSendComplex(channelID, data)
		if err != nil {
			return messages, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func EditMessage(channelID, messageID, content string) (message *discordgo.Message, err error) {
	message, err = cache.GetSession().ChannelMessageEdit(channelID, messageID, content)
	content = CleanDiscordContent(content)
	if err != nil {
		return nil, err
	} else {
		return message, err
	}
}

func EditEmbed(channelID, messageID string, embed *discordgo.MessageEmbed) (message *discordgo.Message, err error) {
	message, err = cache.GetSession().ChannelMessageEditEmbed(channelID, messageID, TruncateEmbed(embed))
	if err != nil {
		return nil, err
	} else {
		return message, err
	}
}

func EditComplex(data *discordgo.MessageEdit) (message *discordgo.Message, err error) {
	if data.Embed != nil {
		data.Embed = TruncateEmbed(data.Embed)
	}
	if data.Content != nil {
		content := CleanDiscordContent(*data.Content)
		data.Content = &content
	}
	message, err = cache.GetSession().ChannelMessageEditComplex(data)
	if err != nil {
		return nil, err
	} else {
		return message, err
	}
}

func CleanDiscordContent(content string) (output string) {
	return strings.Replace(content, "@everyone", "@"+ZERO_WIDTH_SPACE+"everyone", -1)
}

// Applies Embed Limits to the given Embed
// Source: https://discordapp.com/developers/docs/resources/channel#embed-limits
func TruncateEmbed(embed *discordgo.MessageEmbed) (result *discordgo.MessageEmbed) {
	if embed == nil || (&discordgo.MessageEmbed{}) == embed {
		return nil
	}
	if embed.Title != "" && len(embed.Title) > 256 {
		embed.Title = embed.Title[0:255] + "…"
	}
	if len(embed.Description) > 2048 {
		embed.Description = embed.Description[0:2047] + "…"
	}
	if embed.Footer != nil && len(embed.Footer.Text) > 2048 {
		embed.Footer.Text = embed.Footer.Text[0:2047] + "…"
	}
	if embed.Author != nil && len(embed.Author.Name) > 256 {
		embed.Author.Name = embed.Author.Name[0:255] + "…"
	}
	newFields := make([]*discordgo.MessageEmbedField, 0)
	for _, field := range embed.Fields {
		if field.Value == "" {
			continue
		}
		if len(field.Name) > 256 {
			field.Name = field.Name[0:255] + "…"
		}
		// TODO: better cutoff (at commas and stuff)
		if len(field.Value) > 1024 {
			field.Value = field.Value[0:1023] + "…"
		}
		newFields = append(newFields, field)
		if len(newFields) >= 25 {
			break
		}
	}
	embed.Fields = newFields

	if CalculateFullEmbedLength(embed) > 6000 {
		if embed.Footer != nil {
			embed.Footer.Text = ""
		}
		if CalculateFullEmbedLength(embed) > 6000 {
			if embed.Author != nil {
				embed.Author.Name = ""
			}
			if CalculateFullEmbedLength(embed) > 6000 {
				embed.Fields = []*discordgo.MessageEmbedField{{}}
			}
		}
	}

	result = embed
	return result
}

func CalculateFullEmbedLength(embed *discordgo.MessageEmbed) (count int) {
	count += len(embed.Title)
	count += len(embed.Description)
	if embed.Footer != nil {
		count += len(embed.Footer.Text)
	}
	if embed.Author != nil {
		count += len(embed.Author.Name)
	}
	for _, field := range embed.Fields {
		count += len(field.Name)
		count += len(field.Value)
	}
	return count
}

func StartTypingLoop(channelID string) (quitChannel chan int) {
	quitChannel = make(chan int, 1)
	go typingLoop(channelID, quitChannel)
	return quitChannel
}

func typingLoop(channelID string, quitChannel chan int) {
	for {
		select {
		case <-quitChannel:
			return
		default:
			cache.GetSession().ChannelTyping(channelID)
			time.Sleep(5 * time.Second)
		}
	}
}

func ChannelPermissionsInSync(childChannelID string) (inSync bool) {
	childChannel, err := GetChannel(childChannelID)
	if err != nil {
		return false
	}

	if childChannel.ParentID == "" {
		return false
	}

	parentChannel, err := GetChannel(childChannel.ParentID)
	if err != nil {
		return false
	}

	return ChannelOverwritesMatch(parentChannel.PermissionOverwrites, childChannel.PermissionOverwrites)
}

func ChannelOverwritesMatch(aOverwrites, bOverwrites []*discordgo.PermissionOverwrite) (match bool) {
	slice.Sort(aOverwrites, func(i, j int) bool {
		if strings.Compare(aOverwrites[i].ID, aOverwrites[j].ID) > 0 {
			return true
		}
		return false
	})
	slice.Sort(bOverwrites, func(i, j int) bool {
		if strings.Compare(bOverwrites[i].ID, bOverwrites[j].ID) > 0 {
			return true
		}
		return false
	})

	if len(aOverwrites) != len(bOverwrites) {
		return false
	}

	for i := 0; i < len(aOverwrites); i++ {
		if aOverwrites[i].ID != bOverwrites[i].ID {
			return false
		}
		if aOverwrites[i].Type != bOverwrites[i].Type {
			return false
		}
		if aOverwrites[i].Allow != bOverwrites[i].Allow {
			return false
		}
		if aOverwrites[i].Deny != bOverwrites[i].Deny {
			return false
		}
	}

	return true
}

func GetMemberPermissions(guildID, userID string) (apermissions int) {
	guild, err := GetGuildWithoutApi(guildID)
	if err != nil {
		return
	}

	member, err := GetGuildMemberWithoutApi(guildID, userID)
	if err != nil {
		return
	}

	if userID == guild.OwnerID {
		apermissions = discordgo.PermissionAll
		return
	}

	for _, role := range guild.Roles {
		if role.ID == guild.ID {
			apermissions |= role.Permissions
			break
		}
	}

	for _, role := range guild.Roles {
		for _, roleID := range member.Roles {
			if role.ID == roleID {
				apermissions |= role.Permissions
				break
			}
		}
	}

	if apermissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		apermissions |= discordgo.PermissionAll
	}

	return apermissions
}

func GetStaffUsernamesText() (text string) {
	staffList := append(botAdmins, RobyulMod...)
	for i, staffMemberID := range staffList {
		member, err := GetUser(staffMemberID)
		if err == nil {
			text += "`" + member.Username + "#" + member.Discriminator + "`"
			if i < len(staffList)-1 {
				if i < len(staffList)-2 {
					text += ", "
				} else {
					text += " or "
				}
			}
		}
	}
	return
}

func EscapeLinkForMarkdown(input string) (result string) {
	return strings.Replace(strings.Replace(input, ")", "%29", -1), "(", "%28", -1)
}

func IsSnowflake(input string) (snowflake bool) {
	if snowflakeRegex.MatchString(input) {
		return true
	}
	return false
}

// Extracts ALL invites codes from the given message (message can contain multiple invites) (e.g. discord.gg/foo => [foo])
func ExtractInviteCodes(input string) (inviteCodes []string) {
	inviteCodes = make([]string, 0)
	results := discordInviteRegex.FindAllStringSubmatch(input, -1)
	for _, result := range results {
		if len(result) >= 6 {
			inviteCodes = append(inviteCodes, result[5])
		}
	}

	return inviteCodes
}

// Returns an channelID for bot announcements, if possible the default channel when a guild got created, if not the first channel from the top with chat permission
// guildID	: the target Guild to get the channel for
func GetGuildDefaultChannel(guildID string) (channelID string, err error) {
	// get guild
	guild, err := GetGuild(guildID)

	// try System Channel (builtin join and leave messages)
	if guild.SystemChannelID != "" {
		channel, err := GetChannel(guild.SystemChannelID)
		if err == nil && channel.Type == discordgo.ChannelTypeGuildText {
			channelPermissions, err := cache.GetSession().State.UserChannelPermissions(cache.GetSession().State.User.ID, channel.ID)
			if err == nil {
				if channelPermissions&discordgo.PermissionSendMessages == discordgo.PermissionSendMessages {
					return channel.ID, nil
				}
			}
		}
	}

	/*
		// try Widget Channel
		if guild.WidgetChannelID != "" {
			channel, err := GetChannel(guild.WidgetChannelID)
			if err == nil && channel.Type == discordgo.ChannelTypeGuildText {
				channelPermissions, err := cache.GetSession().State.UserChannelPermissions(cache.GetSession().State.User.ID, channel.ID)
				if err == nil {
					if channelPermissions&discordgo.PermissionSendMessages == discordgo.PermissionSendMessages {
						return channel.ID, nil
					}
				}
			}
		}
	*/

	// check channel with the same ID as the guild, the default channel when a guild is being created
	channel, err := GetChannel(guildID)
	if err == nil && channel.Type == discordgo.ChannelTypeGuildText {
		channelPermissions, err := cache.GetSession().State.UserChannelPermissions(cache.GetSession().State.User.ID, channel.ID)
		if err == nil {
			if channelPermissions&discordgo.PermissionSendMessages == discordgo.PermissionSendMessages {
				return channel.ID, nil
			}
		}
	}

	guildChannels := guild.Channels

	// sort guild channels by position
	slice.Sort(guildChannels, func(i, j int) bool {
		return guildChannels[i].Position < guildChannels[j].Position
	})

	// check each channel from the top for a channel with chat permission
	for _, guildChannel := range guildChannels {
		if guildChannel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		channelPermissions, err := cache.GetSession().State.UserChannelPermissions(cache.GetSession().State.User.ID, guildChannel.ID)
		if err == nil {
			if channelPermissions&discordgo.PermissionSendMessages == discordgo.PermissionSendMessages {
				return guildChannel.ID, nil
			}
		}
	}

	// return an error if no channel is found
	return "", errors.New("no default channel found")
}

// DeleteMessageWithDelay will delete the given message after a given time duration
func DeleteMessageWithDelay(msg *discordgo.Message, delay time.Duration) (err error) {
	if msg == nil {
		return nil
	}

	time.Sleep(delay)
	return cache.GetSession().ChannelMessageDelete(msg.ChannelID, msg.ID)
}

type InviteWithCounts struct {
	Guild                    *discordgo.Guild    `json:"guild"`
	Channel                  *discordgo.Channel  `json:"channel"`
	Inviter                  *discordgo.User     `json:"inviter"`
	Code                     string              `json:"code"`
	CreatedAt                discordgo.Timestamp `json:"created_at"`
	MaxAge                   int                 `json:"max_age"`
	Uses                     int                 `json:"uses"`
	MaxUses                  int                 `json:"max_uses"`
	XkcdPass                 string              `json:"xkcdpass"`
	Revoked                  bool                `json:"revoked"`
	Temporary                bool                `json:"temporary"`
	ApproximateMemberCount   int                 `json:"approximate_member_count"`
	ApproximatePresenceCount int                 `json:"approximate_presence_count"`
}

func GetInviteWithCounts(inviteCode string) (invite *InviteWithCounts, err error) {
	respBody, err := cache.GetSession().RequestWithBucketID(
		"GET", discordgo.EndpointInvite(inviteCode)+"?with_counts=true",
		nil, discordgo.EndpointInvite(""),
	)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respBody, &invite)
	if err != nil {
		return nil, err
	}

	return invite, nil
}

// ReplaceEmojis, replaces emoji mentions with text
func ReplaceEmojis(content string) (result string) {
	var replaceWith string

	emojiPartsList := MentionRegexStrict.FindAllStringSubmatch(content, -1)
	if len(emojiPartsList) > 0 {
		for _, emojiParts := range emojiPartsList {
			replaceWith = ":" + emojiParts[1] + ":"

			if replaceWith != "" {
				content = strings.Replace(content, emojiParts[0], replaceWith, -1)
			}
		}
	}

	return content
}
