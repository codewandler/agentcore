package harness

import "github.com/codewandler/agentsdk/command"

// CommandCatalogEntry describes one executable command together with its input
// schema projection. The descriptor remains the source of truth; InputSchema is
// derived from Descriptor.Input.
type CommandCatalogEntry struct {
	Descriptor  command.Descriptor `json:"descriptor"`
	InputSchema command.JSONSchema `json:"inputSchema"`
}

// CommandCatalog returns a flattened catalog of executable commands exposed by
// the session. Non-executable namespace nodes are omitted, but executable parent
// nodes with subcommands would be included.
func (s *Session) CommandCatalog() []CommandCatalogEntry {
	if s == nil {
		return nil
	}
	return commandCatalogFromDescriptors(s.CommandDescriptors())
}

func commandCatalogFromDescriptors(descriptors []command.Descriptor) []CommandCatalogEntry {
	var out []CommandCatalogEntry
	for _, desc := range descriptors {
		appendCommandCatalogEntries(&out, desc)
	}
	return out
}

func appendCommandCatalogEntries(out *[]CommandCatalogEntry, desc command.Descriptor) {
	if desc.Executable {
		*out = append(*out, CommandCatalogEntry{
			Descriptor:  desc,
			InputSchema: command.CommandInputSchema(desc),
		})
	}
	for _, sub := range desc.Subcommands {
		appendCommandCatalogEntries(out, sub)
	}
}
