package discord_bot

import (
	"errors"
	"fmt"
	"log"
	"stable_diffusion_bot/imagine_queue"
	"stable_diffusion_bot/stable_diffusion_api"

	"github.com/bwmarrin/discordgo"
)

type botImpl struct {
	botSession         *discordgo.Session
	guildID            string
	imagineQueue       imagine_queue.Queue
	registeredCommands []*discordgo.ApplicationCommand
}

type Config struct {
	BotToken           string
	GuildID            string
	StableDiffusionAPI stable_diffusion_api.StableDiffusionAPI
}

func New(cfg Config) (Bot, error) {
	if cfg.BotToken == "" {
		return nil, errors.New("missing bot token")
	}

	if cfg.GuildID == "" {
		return nil, errors.New("missing guild ID")
	}

	if cfg.StableDiffusionAPI == nil {
		return nil, errors.New("missing stable diffusion API")
	}

	botSession, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, err
	}

	botSession.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	err = botSession.Open()
	if err != nil {
		return nil, err
	}

	imagineQueue, err := imagine_queue.New(imagine_queue.Config{
		BotSession:         botSession,
		StableDiffusionAPI: cfg.StableDiffusionAPI,
	})
	if err != nil {
		return nil, err
	}

	bot := &botImpl{
		botSession:         botSession,
		imagineQueue:       imagineQueue,
		registeredCommands: make([]*discordgo.ApplicationCommand, 0),
	}

	err = bot.addImagineCommand()
	if err != nil {
		return nil, err
	}

	botSession.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.ApplicationCommandData().Name {
		case "imagine":
			bot.processImagineCommand(s, i)
		default:
			log.Printf("Unknown command '%v'", i.ApplicationCommandData().Name)
		}
	})

	return bot, nil
}

func (b *botImpl) Start() {
	b.imagineQueue.StartPolling()

	err := b.teardown()
	if err != nil {
		log.Printf("Error tearing down bot: %v", err)
	}
}

func (b *botImpl) teardown() error {
	for _, v := range b.registeredCommands {
		log.Printf("Removing command '%s'...", v.Name)

		err := b.botSession.ApplicationCommandDelete(b.botSession.State.User.ID, b.guildID, v.ID)
		if err != nil {
			log.Printf("Error deleting '%v' command: %v", v.Name, err)
		}
	}

	return b.botSession.Close()
}

func (b *botImpl) addImagineCommand() error {
	log.Printf("Adding command 'imagine'...")

	cmd, err := b.botSession.ApplicationCommandCreate(b.botSession.State.User.ID, b.guildID, &discordgo.ApplicationCommand{
		Name:        "imagine",
		Description: "Ask the bot to imagine something",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "prompt",
				Description: "The text prompt to imagine",
				Required:    true,
			},
		},
	})
	if err != nil {
		log.Printf("Error creating '%s' command: %v", cmd.Name, err)

		return err
	}

	b.registeredCommands = append(b.registeredCommands, cmd)

	return nil
}

func (b *botImpl) processImagineCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	var position int
	var queueError error
	var prompt string

	if option, ok := optionMap["prompt"]; ok {
		prompt = option.StringValue()

		position, queueError = b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
			Prompt:             prompt,
			DiscordInteraction: i.Interaction,
		})
		if queueError != nil {
			log.Printf("Error adding imagine to queue: %v\n", queueError)
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		// Ignore type for now, they will be discussed in "responses"
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(
				"I'm dreaming something up for you. You are currently #%d in line.\n<@%s> asked me to imagine \"%s\".",
				position,
				i.Member.User.ID,
				prompt),
		},
	})
}
