package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

/*
Cmd represents a command (or sub command) in a CLI application. It should be constructed
by calling Command() on an app to create a top level command or by calling Command() on another
command to create a sub command
*/
type Cmd struct {
	// The code to execute when this command is matched
	Action func()
	// The code to execute before this command or any of its children is matched
	Before func()
	// The code to execute after this command or any of its children is matched
	After func()
	// The command options and arguments
	Spec string
	// The command error handling strategy
	ErrorHandling flag.ErrorHandling

	init CmdInitializer
	name string
	desc string

	commands   []*Cmd
	options    []*opt
	optionsIdx map[string]*opt
	args       []*arg
	argsIdx    map[string]*arg

	parents []string

	fsm *state
}

/*
BoolParam represents a Bool option or argument
*/
type BoolParam interface{}

/*
StringParam represents a String option or argument
*/
type StringParam interface{}

/*
IntParam represents an Int option or argument
*/
type IntParam interface{}

/*
StringsParam represents a string slice option or argument
*/
type StringsParam interface{}

/*
IntsParam represents an int slice option or argument
*/
type IntsParam interface{}

/*
CmdInitializer is a function that configures a command by adding options, arguments, a spec, sub commands and the code
to execute when the command is called
*/
type CmdInitializer func(*Cmd)

/*
Command adds a new (sub) command to c where name is the command name (what you type in the console),
description is what would be shown in the help messages, e.g.:

	Usage: git [OPTIONS] COMMAND [arg...]

	Commands:
	  $name	$desc

the last argument, init, is a function that will be called by mow.cli to further configure the created
(sub) command, e.g. to add options, arguments and the code to execute
*/
func (c *Cmd) Command(name, desc string, init CmdInitializer) {
	c.commands = append(c.commands, &Cmd{
		ErrorHandling: c.ErrorHandling,
		name:          name,
		desc:          desc,
		init:          init,
		commands:      []*Cmd{},
		options:       []*opt{},
		optionsIdx:    map[string]*opt{},
		args:          []*arg{},
		argsIdx:       map[string]*arg{},
	})
}

/*
Bool can be used to add a bool option or argument to a command.
It accepts either a BoolOpt or a BoolArg struct.

The result should be stored in a variable (a pointer to a bool) which will be populated when the app is run and the call arguments get parsed
*/
func (c *Cmd) Bool(p BoolParam) *bool {
	switch x := p.(type) {
	case BoolOpt:
		return c.mkOpt(opt{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*bool)
	case BoolArg:
		return c.mkArg(arg{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*bool)
	default:
		panic(fmt.Sprintf("Unhandled param %v", p))
	}
}

/*
String can be used to add a string option or argument to a command.
It accepts either a StringOpt or a StringArg struct.

The result should be stored in a variable (a pointer to a string) which will be populated when the app is run and the call arguments get parsed
*/
func (c *Cmd) String(p StringParam) *string {
	switch x := p.(type) {
	case StringOpt:
		return c.mkOpt(opt{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*string)
	case StringArg:
		return c.mkArg(arg{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*string)
	default:
		panic(fmt.Sprintf("Unhandled param %v", p))
	}
}

/*
Int can be used to add an int option or argument to a command.
It accepts either a IntOpt or a IntArg struct.

The result should be stored in a variable (a pointer to an int) which will be populated when the app is run and the call arguments get parsed
*/
func (c *Cmd) Int(p IntParam) *int {
	switch x := p.(type) {
	case IntOpt:
		return c.mkOpt(opt{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*int)
	case IntArg:
		return c.mkArg(arg{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*int)
	default:
		panic(fmt.Sprintf("Unhandled param %v", p))
	}
}

/*
Strings can be used to add a string slice option or argument to a command.
It accepts either a StringsOpt or a StringsArg struct.

The result should be stored in a variable (a pointer to a string slice) which will be populated when the app is run and the call arguments get parsed
*/
func (c *Cmd) Strings(p StringsParam) *[]string {
	switch x := p.(type) {
	case StringsOpt:
		return c.mkOpt(opt{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*[]string)
	case StringsArg:
		return c.mkArg(arg{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*[]string)
	default:
		panic(fmt.Sprintf("Unhandled param %v", p))
	}
}

/*
Ints can be used to add an int slice option or argument to a command.
It accepts either a IntsOpt or a IntsArg struct.

The result should be stored in a variable (a pointer to an int slice) which will be populated when the app is run and the call arguments get parsed
*/
func (c *Cmd) Ints(p IntsParam) *[]int {
	switch x := p.(type) {
	case IntsOpt:
		return c.mkOpt(opt{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*[]int)
	case IntsArg:
		return c.mkArg(arg{name: x.Name, desc: x.Desc, envVar: x.EnvVar, hideValue: x.HideValue}, x.Value).(*[]int)
	default:
		panic(fmt.Sprintf("Unhandled param %v", p))
	}
}

func (c *Cmd) doInit() error {
	if c.init != nil {
		c.init(c)
	}

	parents := append(c.parents, c.name)

	for _, sub := range c.commands {
		sub.parents = parents
	}

	if len(c.Spec) == 0 {
		if len(c.options) > 0 {
			c.Spec = "[OPTIONS] "
		}
		for _, arg := range c.args {
			c.Spec += arg.name + " "
		}
	}
	fsm, err := uParse(c)
	if err != nil {
		return err
	}
	c.fsm = fsm
	return nil
}

func (c *Cmd) onError(err error) {
	if err != nil {
		switch c.ErrorHandling {
		case flag.ExitOnError:
			exiter(2)
		case flag.PanicOnError:
			panic(err)
		}
	} else {
		if c.ErrorHandling == flag.ExitOnError {
			exiter(2)
		}
	}
}

/*
PrintHelp prints the command's help message.
In most cases the library users won't need to call this method, unless
a more complex validation is needed
*/
func (c *Cmd) PrintHelp() {
	out := os.Stderr

	full := append(c.parents, c.name)
	path := strings.Join(full, " ")
	fmt.Fprintf(out, "\nUsage: %s", path)

	spec := strings.TrimSpace(c.Spec)
	if len(spec) > 0 {
		fmt.Fprintf(out, " %s", spec)
	}

	if len(c.commands) > 0 {
		fmt.Fprint(out, " COMMAND [arg...]")
	}
	fmt.Fprint(out, "\n\n")

	if len(c.desc) > 0 {
		fmt.Fprintf(out, "%s\n", c.desc)
	}

	w := tabwriter.NewWriter(out, 15, 1, 3, ' ', 0)

	if len(c.args) > 0 {
		fmt.Fprintf(out, "\nArguments:\n")

		for _, arg := range c.args {
			desc := c.formatDescription(arg.desc, arg.envVar)
			value := c.formatArgValue(arg)

			fmt.Fprintf(w, "  %s%s\t%s\n", arg.name, value, desc)
		}
		w.Flush()
	}

	if len(c.options) > 0 {
		fmt.Fprintf(out, "\nOptions:\n")

		for _, opt := range c.options {
			desc := c.formatDescription(opt.desc, opt.envVar)
			value := c.formatOptValue(opt)
			fmt.Fprintf(w, "  %s%s\t%s\n", strings.Join(opt.names, ", "), value, desc)
		}
		w.Flush()
	}

	if len(c.commands) > 0 {
		fmt.Fprintf(out, "\nCommands:\n")

		for _, c := range c.commands {
			fmt.Fprintf(w, "  %s\t%s\n", c.name, c.desc)
		}
		w.Flush()
	}

	if len(c.commands) > 0 {
		fmt.Fprintf(out, "\nRun '%s COMMAND --help' for more information on a command.\n", path)
	}
}

func (c *Cmd) formatArgValue(arg *arg) string {
	if arg.hideValue {
		return " "
	}
	return "=" + arg.helpFormatter(arg.get())
}

func (c *Cmd) formatOptValue(opt *opt) string {
	if opt.hideValue {
		return " "
	}
	return "=" + opt.helpFormatter(opt.get())
}

func (c *Cmd) formatDescription(desc, envVar string) string {
	var b bytes.Buffer
	b.WriteString(desc)
	if len(envVar) > 0 {
		b.WriteString(" (")
		sep := ""
		for _, envVal := range strings.Split(envVar, " ") {
			b.WriteString(fmt.Sprintf("%s$%s", sep, envVal))
			sep = " "
		}
		b.WriteString(")")
	}
	return strings.TrimSpace(b.String())
}

func (c *Cmd) parse(args []string, entry, inFlow, outFlow *step) error {
	if c.helpRequested(args) {
		c.PrintHelp()
		c.onError(nil)
		return nil
	}

	nargsLen := c.getOptsAndArgs(args)

	if err := c.fsm.parse(args[:nargsLen]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		c.PrintHelp()
		c.onError(err)
		return err
	}

	newInFlow := &step{
		do:    c.Before,
		error: outFlow,
		desc:  fmt.Sprintf("%s.Before", c.name),
	}
	inFlow.success = newInFlow

	newOutFlow := &step{
		do:      c.After,
		success: outFlow,
		error:   outFlow,
		desc:    fmt.Sprintf("%s.After", c.name),
	}

	args = args[nargsLen:]
	if len(args) == 0 {
		if c.Action != nil {
			newInFlow.success = &step{
				do:      c.Action,
				success: newOutFlow,
				error:   newOutFlow,
				desc:    fmt.Sprintf("%s.Action", c.name),
			}

			entry.run(nil)
			return nil
		}
		c.PrintHelp()
		c.onError(nil)
		return nil
	}

	arg := args[0]
	for _, sub := range c.commands {
		if arg == sub.name {
			if err := sub.doInit(); err != nil {
				panic(err)
			}
			return sub.parse(args[1:], entry, newInFlow, newOutFlow)
		}
	}

	var err error
	switch {
	case strings.HasPrefix(arg, "-"):
		err = fmt.Errorf("Error: illegal option %s", arg)
		fmt.Fprintln(os.Stderr, err.Error())
	default:
		err = fmt.Errorf("Error: illegal input %s", arg)
		fmt.Fprintln(os.Stderr, err.Error())
	}
	c.PrintHelp()
	c.onError(err)
	return err

}

func (c *Cmd) isArgSet(args []string, searchArgs []string) bool {
	for _, arg := range args {
		for _, sub := range c.commands {
			if arg == sub.name {
				return false
			}
		}
		for _, searchArg := range searchArgs {
			if arg == searchArg {
				return true
			}
		}
	}
	return false
}

func (c *Cmd) helpRequested(args []string) bool {
	return c.isArgSet(args, []string{"-h", "--help"})
}

func (c *Cmd) getOptsAndArgs(args []string) int {
	consumed := 0

	for _, arg := range args {
		for _, sub := range c.commands {
			if arg == sub.name {
				return consumed
			}
		}
		consumed++
	}
	return consumed
}
