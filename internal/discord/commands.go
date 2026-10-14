package discord

import "github.com/bwmarrin/discordgo"

// Commands defines all application slash commands exposed by Sköll.
var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        "link",
		Description: "Link your Discord identity to Sköll and verify role authorizations",
	},
	{
		Name:        "status",
		Description: "View Fenrir Daemon host metrics and active container stacks",
	},
	{
		Name:        "server",
		Description: "Control container stack lifecycle (Operator+)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "restart",
				Description: "Restart all containers in a named stack",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Name of the stack to restart (e.g. minecraft-prod)",
						Required:    true,
					},
				},
			},
			{
				Name:        "start",
				Description: "Start up a named stack",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Name of the stack to start",
						Required:    true,
					},
				},
			},
			{
				Name:        "stop",
				Description: "Stop a named stack",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Name of the stack to stop",
						Required:    true,
					},
				},
			},
		},
	},
	{
		Name:        "stack",
		Description: "Stack deployment and infrastructure management (Admin)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "deploy",
				Description: "Deploy or update a Docker Compose stack (Admin only)",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Target stack name",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "compose",
						Description: "Docker Compose YAML string content",
						Required:    true,
					},
				},
			},
			{
				Name:        "list",
				Description: "List all deployed stacks and their runtime states",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	},
}
