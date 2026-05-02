# Command Tree Direction

## Problem

The current command implementation is intentionally simple, but it should not be the long-term SDK surface. Harness commands such as `/workflow` currently hide subcommands, positional arguments, flags, and validation inside handwritten switch statements and ad hoc parsing. That creates several problems:

- subcommands are invisible to the type system;
- command help, docs, and API schemas cannot be generated reliably;
- flags and positional arguments are repeatedly hand-parsed;
- terminal slash commands, HTTP command APIs, and LLM-callable command projections would each need bespoke mapping;
- every new command namespace increases later migration cost.

Until this is addressed, avoid adding more broad command namespaces in the current switch/parse style.

## Direction

Commands should become a declarative, channel-neutral command tree: similar in spirit to Cobra, but smaller and SDK-native. The command package should know about:

- root command names such as `workflow`, `session`, `agent`, or `thread`;
- nested subcommands such as `workflow runs`, `workflow show`, `agent list`;
- positional arguments;
- flags;
- enum/required/default constraints;
- structured input/output metadata;
- descriptors that can later become JSON Schema or OpenAPI-style command APIs.

Terminal slash syntax should be one input projection over this tree, not the canonical command model.

## Target shape

A builder-style API is preferred because it stays readable for SDK consumers:

```go
workflowTree := command.NewTree("workflow").
    Description("Inspect and run workflows").
    Sub("list", workflowListHandler).
    Sub("show", workflowShowHandler,
        command.Arg("name").Required().Description("Workflow name"),
    ).
    Sub("start", workflowStartHandler,
        command.Arg("name").Required(),
        command.Arg("input").Variadic(),
    ).
    Sub("runs", workflowRunsHandler,
        command.Flag("workflow").String().Description("Workflow name"),
        command.Flag("status").Enum("running", "succeeded", "failed"),
    ).
    Sub("run", workflowRunHandler,
        command.Arg("run_id").Required(),
    )
```

A functional style is also acceptable if it fits existing package conventions better:

```go
command.Group(command.Spec{Name: "workflow", Description: "Inspect and run workflows"},
    command.Leaf(command.Spec{Name: "list"}, workflowListHandler),
    command.Leaf(command.Spec{Name: "show"}, workflowShowHandler,
        command.Arg("name", command.Required()),
    ),
    command.Leaf(command.Spec{Name: "runs"}, workflowRunsHandler,
        command.Flag("workflow"),
        command.Flag("status", command.Enum("running", "succeeded", "failed")),
    ),
)
```

The builder API is currently the preferred direction.

## Typed input direction

The first slice may keep handlers as:

```go
type Handler func(context.Context, command.Params) (command.Result, error)
```

but the target should support typed command inputs, similar to `action.NewTyped`:

```go
type WorkflowRunsInput struct {
    Workflow string             `json:"workflow,omitempty" command:"flag=workflow"`
    Status   workflow.RunStatus `json:"status,omitempty" command:"flag=status,enum=running|succeeded|failed"`
}

command.Typed("runs", func(ctx context.Context, in WorkflowRunsInput) (WorkflowRunsPayload, error) {
    // no manual flag parsing
})
```

Commands are not a replacement for actions. The intended relationship is:

```text
Action   = executable typed unit
Command  = user/channel invocation surface over typed input
Tool     = model-callable projection over typed input
Workflow = orchestration over actions
```

Some commands may wrap actions. Other commands, such as `/session info`, are channel/session control surfaces and may not be model-callable actions.

## Descriptor and schema direction

A running harness should eventually be able to expose all supported command shapes through descriptors:

```go
type Descriptor struct {
    Name        string
    Path        []string
    Description string
    Args        []ArgSpec
    Flags       []FlagSpec
    // later: InputType / JSON Schema / output payload metadata
}

func (t *Tree) Descriptors() []Descriptor
```

Example descriptor for `/workflow runs`:

```json
{
  "name": "workflow.runs",
  "path": ["workflow", "runs"],
  "description": "List workflow runs",
  "input": {
    "type": "object",
    "properties": {
      "workflow": { "type": "string" },
      "status": {
        "type": "string",
        "enum": ["running", "succeeded", "failed"]
      }
    }
  }
}
```

This enables future surfaces from the same command model:

- terminal slash commands;
- HTTP command execution APIs;
- web forms;
- generated help/docs;
- LLM-callable command projections where explicitly allowed;
- JSON/machine-readable command invocation.

## Migration plan

Do not keep adding command namespaces with handwritten switch-based subcommand parsing. The next command-related work should be one of:

1. Add the declarative command tree core in `command`.
2. Migrate existing harness command namespaces (`/workflow`, `/session`) onto it.
3. Add command descriptors/introspection.
4. Add typed command input binding.

Recommended commit sequence:

```text
Add declarative command trees
Use command trees for harness commands
Expose command tree descriptors
Add typed command input binding
```

During migration, keep existing terminal behavior stable. The current `command.Parse` tokenizer can remain as the terminal slash syntax parser, but command validation and command metadata should move into the declarative tree.
