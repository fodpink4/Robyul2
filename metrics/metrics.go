package metrics

import (
    "net/http"
    "expvar"
    "github.com/bwmarrin/discordgo"
    "time"
    "github.com/sn0w/Karen/logger"
    "runtime"
)

var (
    // MessagesReceived counts all ever received messages
    MessagesReceived = expvar.NewInt("messages_received")

    // UserCount counts all logged-in users
    UserCount = expvar.NewInt("user_count")

    // ChannelCount counts all watching channels
    ChannelCount = expvar.NewInt("channel_count")

    // GuildCount counts all joined guilds
    GuildCount = expvar.NewInt("guild_count")

    // CommandsExecuted increases after each command execution
    CommandsExecuted = expvar.NewInt("commands_executed")

    // CleverbotRequests increases after each request to cleverbot.com
    CleverbotRequests = expvar.NewInt("cleverbot_requests")

    // CoroutineCount counts all running coroutines
    CoroutineCount = expvar.NewInt("coroutine_count")
)

// Init starts a http server on 127.0.0.1:1337
func Init() {
    logger.INFO.L("metrics", "Listening on http://[::1]:1337")
    go http.ListenAndServe(":1337", nil)
}

// OnReady listens for said discord event
func OnReady(session *discordgo.Session, event *discordgo.Ready) {
    go CollectDiscordMetrics(session)
    go CollectRuntimeMetrics()
}

// OnMessageCreate listens for said discord event
func OnMessageCreate(session *discordgo.Session, event *discordgo.MessageCreate) {
    MessagesReceived.Add(1)
}

// CollectDiscordMetrics counts Guilds, Channels and Users
func CollectDiscordMetrics(session *discordgo.Session) {
    for {
        time.Sleep(15 * time.Second)

        users := make(map[string]string)
        channels := 0
        guilds := session.State.Guilds

        for _, guild := range guilds {
            channels += len(guild.Channels)

            for _, u := range guild.Members {
                users[u.User.ID] = u.User.Username
            }
        }

        UserCount.Set(int64(len(users)))
        ChannelCount.Set(int64(channels))
        GuildCount.Set(int64(len(guilds)))
    }
}

// CollectRuntimeMetrics counts all running coroutines
func CollectRuntimeMetrics() {
    for {
        time.Sleep(15 * time.Second)
        CoroutineCount.Set(int64(runtime.NumGoroutine()))
    }
}