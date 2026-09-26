package cli

import (
	"context"
	"fmt"
	"io"
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
func RunDoctor(ctx context.Context, _ []string, stdout, stderr io.Writer) int {
	_ = config.LoadDotEnv(".env")

	var list Checklist
	env := config.Target()

	if err := env.Validate(); err != nil {
		list.Warn("Configuration", err.Error())
		list.WriteTerminal(stdout, "Appwrite Migration Guard — doctor")
		return list.ExitCode()
	}
	list.Pass("Configuration (APPWRITE_ENDPOINT/PROJECT_ID/API_KEY present)")

	client := appwrite.New(env)
	runCtx, cancel := context.WithTimeout(ctx, doctorTimeout)
	defer cancel()

	v, err := client.Version(runCtx)
	if err != nil {
		list.Block("Endpoint reachable", explain(err))
		list.WriteTerminal(stdout, "Appwrite Migration Guard — doctor")
		return list.ExitCode()
	}
	list.Pass(fmt.Sprintf("Endpoint reachable (Appwrite %s)", v.Version))

	if _, err := client.Health(runCtx); err != nil {
		switch {
		case errs.IsKind(err, errs.KindAuthentication):
			list.Block("Authentication", "API key was rejected: "+explain(err))
		case errs.IsKind(err, errs.KindAuthorization):
			list.Block("Authentication", "API key is missing the health.read scope: "+explain(err))
		default:
			list.Block("Authentication", explain(err))
		}
		list.WriteTerminal(stdout, "Appwrite Migration Guard — doctor")
		return list.ExitCode()
	}
	list.Pass("Authentication (API key accepted, health.read scope confirmed)")

	list.WriteTerminal(stdout, "Appwrite Migration Guard — doctor")
	return list.ExitCode()
}

// explain renders an error for terminal display without ever including
// credential material — internal/appwrite guarantees API keys never reach
// error text (they are sent only as headers), so this is a plain format.
func explain(err error) string {
	return err.Error()
}
