// Except.go: Contains functions to make handling panics less PITA

package helpers

import (
    "reflect"
    "github.com/bwmarrin/discordgo"
    "fmt"
    "runtime"
    "github.com/getsentry/raven-go"
    "strconv"
    "errors"
)

type Callback func()

func RecoverDiscord(session *discordgo.Session, msg *discordgo.Message) {
    err := recover()
    if err != nil {
        SendError(session, msg, err)
    }
}

func Recover() {
    err := recover()
    if err != nil {
        fmt.Printf("%#v", err)
    }
}

// Softer form of relax
// Calls a callback instead of panicking
func SoftRelax(err error, cb Callback) {
    if err != nil {
        cb()
    }
}

// Helper to reduce if-checks if panicking is allowed
// If $err is nil this is a no-op. Panics otherwise.
func Relax(err error) {
    if err != nil {
        panic(err)
    }
}

// if a != b throw err
func RelaxAssertEqual(a interface{}, b interface{}, err error) {
    if !reflect.DeepEqual(a, b) {
        Relax(err)
    }
}

// if a == b throw err
func RelaxAssertUnequal(a interface{}, b interface{}, err error) {
    if reflect.DeepEqual(a, b) {
        Relax(err)
    }
}

// Takes an error and sends it to discord and sentry.io
func SendError(session *discordgo.Session, msg *discordgo.Message, err interface{}) {
    if DEBUG_MODE == true {
        buf := make([]byte, 1 << 16)
        stackSize := runtime.Stack(buf, false)

        session.ChannelMessageSend(
            msg.ChannelID,
            "Error :frowning:\n0xFADED#3237 has been notified.\n```\n" +
                fmt.Sprintf("%#v\n", err) +
                fmt.Sprintf("%s\n", string(buf[0:stackSize])) +
                "\n```\nhttp://i.imgur.com/FcV2n4X.jpg",
        )
    } else {
        session.ChannelMessageSend(
            msg.ChannelID,
            "Error :frowning:\n0xFADED#3237 has been notified.\n```\n" +
                fmt.Sprintf("%#v", err) +
                "\n```\nhttp://i.imgur.com/FcV2n4X.jpg",
        )
    }

    raven.SetUserContext(&raven.User{
        ID:       msg.ID,
        Username: msg.Author.Username + "#" + msg.Author.Discriminator,
    })
    raven.CaptureError(errors.New(fmt.Sprintf("%#v", err)), map[string]string{
        "ChannelID":       msg.ChannelID,
        "Content":         msg.Content,
        "Timestamp":       string(msg.Timestamp),
        "TTS":             strconv.FormatBool(msg.Tts),
        "MentionEveryone": strconv.FormatBool(msg.MentionEveryone),
        "IsBot":           strconv.FormatBool(msg.Author.Bot),
    })
}
