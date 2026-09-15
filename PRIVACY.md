# Privacy policy

This policy covers the Meshtastic Discord bot, the application that provides the
`/bug`, `/feature`, `/faq`, `/changelog`, `/repo`, and `/tapsign` commands. It
does not cover the Meshtastic firmware, the client applications, or the
Meshtastic Discord server itself.

## What the bot collects

The bot collects information only when you run one of its commands. It does not
read channel messages: it registers no handler for message events and acts only
on the slash commands listed above.

### `/bug` and `/feature`

These commands open a form. When you submit it, the bot creates an issue in a
public GitHub repository containing:

- every value you typed into the form
- your Discord username and your Discord numeric user ID

**Both are published publicly and permanently.** The issue is visible to anyone
on the internet, is indexed by search engines, and is copied by third parties
that mirror GitHub data. Removing it later does not undo that.

The bot adds a footer to each issue in this form:

```
---
Submitted via Discord by: <username> (<user ID>)
```

### `/faq`, `/changelog`, `/repo`, and `/tapsign`

These commands read public information and reply to you. Nothing is retained.

## What the bot does not collect

- No database, no file storage, and no analytics. The bot stores nothing between
  restarts.
- No channel messages, direct messages, voice data, or attachments.
- No advertising, profiling, or sale of information to anyone.

## Retention

A form with more than five fields is collected across several dialogs, because
Discord limits a dialog to five fields. While you are filling one in, your
answers are held in the bot's memory so the next dialog can add to them. That
copy is discarded when the issue is created, when 30 minutes pass without
completing it, or when the bot restarts.

Nothing identifying is written to the machine the bot runs on.

After the issue is created, your submission lives in GitHub, not with the bot.

## Third parties

- **Discord** delivers your commands to the bot and is governed by its own
  [privacy policy](https://discord.com/privacy).
- **GitHub** hosts the issues the bot creates and is governed by the
  [GitHub Privacy Statement](https://docs.github.com/site-policy/privacy-policies/github-privacy-statement).

No other service receives your information.

## Removing what you submitted

Because the bot keeps no database, removal means removing the GitHub issue or
comment. Ask a maintainer in the Meshtastic Discord server, naming the issue you
want removed. Maintainers can delete or edit it.

Deletion removes the issue from GitHub. It cannot recall copies already made by
search engines, mirrors, or anyone who read it while it was public.

Information held by Discord about your account is controlled by Discord, not by
this project.

## Children

The bot is not directed at children. It collects only what you type into a form
and the Discord username attached to it.

## Changes

Changes to this policy are made in this file, and its history is public in this
repository.

## Contact

Open an issue on
[meshtastic/meshtastic-bot](https://github.com/meshtastic/meshtastic-bot/issues).
