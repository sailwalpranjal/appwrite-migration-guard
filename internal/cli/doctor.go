package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

const doctorTimeout = 10 * time.Second

// RunDoctor implements `amg doctor`: it verifies that amg is configured
// correctly and, if credentials are present, that the target Appwrite
// endpoint is reachable and the API key is valid. It never mutates
// anything and never requires a destination environment.
func RunDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	timeout := fs.Duration("timeout", doctorTimeout, "maximum time to allow the reachability/auth check — raise this for a high-latency self-hosted endpoint")
	jsonOut := fs.Bool("json", false, "print the checklist as JSON instead of a terminal summary — feed the saved file to `amg report` for text/json/html rendering")
	if err := fs.Parse(args); err != nil {
		return exitForParseError(err)
	}

	_ = config.LoadDotEnv(".env")

	var list Checklist
	env := config.Target()

	if err := env.Validate(); err != nil {
		list.Warn("Configuration", err.Error())
		return finishChecklist(&list, "doctor", "Appwrite Migration Guard — doctor", *jsonOut, stdout, stderr)
	}
	list.Pass("Configuration (APPWRITE_ENDPOINT/PROJECT_ID/API_KEY present)")

	client := appwrite.New(env)
	runCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	v, err := client.Version(runCtx)
	if err != nil {
		list.Block("Endpoint reachable", explainReachabilityError(err))
		return finishChecklist(&list, "doctor", "Appwrite Migration Guard — doctor", *jsonOut, stdout, stderr)
	}
	list.Pass(fmt.Sprintf("Endpoint reachable (Appwrite %s)", v.Version))

	if _, err := client.Health(runCtx); err != nil {
		list.Block("Authentication", explainAuthError(err))
		return finishChecklist(&list, "doctor", "Appwrite Migration Guard — doctor", *jsonOut, stdout, stderr)
	}
	list.Pass("Authentication (API key accepted, health.read scope confirmed)")

	return finishChecklist(&list, "doctor", "Appwrite Migration Guard — doctor", *jsonOut, stdout, stderr)
}

// finishChecklist renders list as JSON or terminal text (every early
// return in doctor/preflight goes through this single exit point, so
// --json behaves identically regardless of which check ended the run)
// and returns the resulting exit code.
func finishChecklist(list *Checklist, command, title string, jsonOut bool, stdout, stderr io.Writer) int {
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(list.ToResult(command)); err != nil {
			fmt.Fprintln(stderr, "amg "+command+": encode JSON:", err.Error())
			return ExitBlock
		}
		return list.ExitCode()
	}
	list.WriteTerminal(stdout, title)
	return list.ExitCode()
}

// explain renders an error for terminal display without ever including
// credential material — internal/appwrite guarantees API keys never reach
// error text (they are sent only as headers), so this is a plain format.
func explain(err error) string {
	return err.Error()
}

// explainReachabilityError renders a Client.Version() failure with a
// specific hint when the error is Appwrite Cloud's regional-routing 401
// ("Project is not accessible in this region..."), rather than a bare
// error string. Version() never sends the API key (see requestAuth's doc
// comment), so a 401 here is essentially never a credentials problem —
// verified live against Appwrite Cloud 2.3.0 by pointing a valid
// project/key pair at the wrong region's endpoint (e.g. fra instead of
// nyc), which reproduces exactly this response. Appwrite's own message
// already names the cause; this only adds the concrete fix.
func explainReachabilityError(err error) string {
	msg := explain(err)
	if errs.IsKind(err, errs.KindAuthentication) && strings.Contains(strings.ToLower(msg), "region") {
		return msg + " — APPWRITE_ENDPOINT most likely points at the wrong Appwrite Cloud region for this project (e.g. fra vs nyc vs syd); open the project's Overview page in the Appwrite console and copy its API Endpoint exactly."
	}
	return msg
}

// explainAuthError renders a Client.Health() failure with a specific,
// actionable reason where amg can tell one apart from another, instead of
// a bare error string — shared by doctor and preflight so the same
// underlying failure gets the same explanation in both commands.
func explainAuthError(err error) string {
	switch {
	case errs.IsKind(err, errs.KindAuthentication):
		return "API key was rejected: " + explain(err)
	case errs.IsKind(err, errs.KindAuthorization):
		return "API key is missing the health.read scope: " + explain(err)
	default:
		return explain(err)
	}
}
