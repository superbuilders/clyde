import { execFileSync } from "node:child_process";

// Run a shell snippet on the deployed box as root, over SSM.
//
// The M4 gate authenticates as Bob, but the operation under test is Alice
// granting — and Bob must not be able to grant to himself, which is the whole
// point of the milestone. So the privileged half runs out of band, exactly as
// the M3 fixture does.
//
// This shells out to the same `bonnie share` subcommand an operator would use,
// so the gate exercises the deployed binary rather than a re-implementation of
// its logic in the test.

const AWS_PROFILE = process.env.AWS_PROFILE ?? "superbuilders-prod";
const REGION = process.env.AWS_REGION ?? "us-east-1";

function aws(args: string[]): string {
  return execFileSync("aws", ["--profile", AWS_PROFILE, "--region", REGION, ...args], {
    encoding: "utf8",
    env: { ...process.env, AWS_PAGER: "" },
  }).trim();
}

let cachedInstance: string | undefined;

export function instanceId(): string {
  if (cachedInstance) return cachedInstance;
  if (process.env.BONNIE_INSTANCE) {
    cachedInstance = process.env.BONNIE_INSTANCE;
    return cachedInstance;
  }
  const out = aws([
    "ec2",
    "describe-instances",
    "--filters",
    "Name=tag:Name,Values=bonnie-dev",
    "Name=instance-state-name,Values=running",
    "--query",
    "Reservations[].Instances[].InstanceId",
    "--output",
    "text",
  ]);
  cachedInstance = out.split(/\s+/)[0];
  if (!cachedInstance) throw new Error("no running instance tagged bonnie-dev");
  return cachedInstance;
}

export interface SsmResult {
  status: string;
  stdout: string;
  stderr: string;
}

export function ssm(script: string, timeoutMs = 120_000): SsmResult {
  const id = aws([
    "ssm",
    "send-command",
    "--instance-ids",
    instanceId(),
    "--document-name",
    "AWS-RunShellScript",
    "--parameters",
    JSON.stringify({ commands: [script] }),
    "--query",
    "Command.CommandId",
    "--output",
    "text",
  ]);

  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const raw = aws([
        "ssm",
        "get-command-invocation",
        "--command-id",
        id,
        "--instance-id",
        instanceId(),
        "--output",
        "json",
      ]);
      const d = JSON.parse(raw);
      if (["Success", "Failed", "TimedOut", "Cancelled"].includes(d.Status)) {
        return {
          status: d.Status,
          stdout: d.StandardOutputContent ?? "",
          stderr: d.StandardErrorContent ?? "",
        };
      }
    } catch {
      // The invocation is not visible immediately after send-command; retry.
    }
    execFileSync("sleep", ["2"]);
  }
  throw new Error("timed out waiting for SSM");
}

/** Run `bonnie share` on the box, returning its combined output. */
export function bonnieShare(args: string): SsmResult {
  return ssm(`/opt/bonnie/current/bonnie share ${args} 2>&1`);
}
