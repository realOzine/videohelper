# Repository Guidelines

## Desktop Scope and Structure

VideoHelper is a personal cross-platform desktop app, with macOS Apple Silicon first. `cmd/videohelper-desktop/` owns startup/lifecycle; `desktop/` hosts MyGo; `web/` contains the embedded React UI. `internal/desktop/` owns SQLite/sqlite-vec, local files, durable jobs, local UI endpoints and private runtime setup. Reuse `internal/media/`, `internal/mlclient/`, `internal/retrieval/` and `internal/timeline/`. Python models remain in `ml/app/`, `ml/plugins/`, and locked `ml/profiles/`. Keep Chinese `docs/design.md`, `docs/spec.md` and the ML contract aligned.

There are no accounts, tenants, JWT, public admin APIs, per-user limits, billing, PostgreSQL, Redis, S3 or Docker dependencies. Keep the desktop focused on responsiveness, clear UI, compatibility and search. The short-lived loopback capability and managed-file boundaries protect local process communication; they are not a user login system.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox. Unless the user explicitly requests work there, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from searches, builds, tests and refactors. General repository work does not authorize access.

## Build and Validation

- `npm --prefix web ci` installs frontend development dependencies.
- `make desktop` / `make build` builds the embedded desktop executable; `make desktop-run` starts MyGo development mode.
- `make test-go`, `make test-web`, `make vet` run scoped checks; do not use an unrestricted repository-wide test command.
- Python checks use the managed development environment in `ml/`: `uv run pytest` and `uv run ruff check .`.
- Real tool/model tests require explicit `VH_TEST_DESKTOP_*` directories. Use disposable task-owned data and preserve useful evidence. Fake-model tests do not establish real retrieval quality.
- Runtime media belongs in the selected application data directory. Repository test artifacts belong in `.local/`; evaluation evidence follows `Eval/AGENTS.md`.

## Implementation Rules

Use gofmt, Python four-space indentation and Ruff. Keep functions focused, reuse existing helpers, add meaningful tests for non-trivial logic. SQL tables use singular snake_case, wire IDs are strings, and time intervals are half-open integer milliseconds.

Preserve original media for ASR and CLAP. Pass query text unchanged. Keep model fingerprints and backend indexes isolated. LLM/Agent remains deferred. A single distributable executable may release components and download models; do not require the user's global Python, FFmpeg, yt-dlp or external services.

## Agent Workflow

- Start implementation in an isolated feature worktree; keep `main` for reviewed changes. Preserve unrelated changes and user data.
- Enable Ponytail FULL for every execution, including sub-agents: minimal complete changes with validation, cancellation and data safety intact.
- Use `gpt-6.1-sol` at high effort for implementation sub-agents. Independent parallel tasks use separate branches/worktrees based on the feature branch.
- Work in small verified units, commit frequently, and integrate checked units into the feature branch promptly.
- Update affected documentation without development journals or redundant acceptance reports.
- Review and verify the combined feature before merging into `main`. Clean merged task worktrees/branches and task-created temporary services after preserving useful validation evidence. Keep the requested feature worktree while work remains there.

<!-- BEGIN AWS Agent Toolkit rules -->
# AWS Guidance for the new AWS experience

This user has signed up for the new AWS experience. This experience lets you sign into AWS using a social provider and requires the following additional context.

Where this guidance conflicts with the project's own instructions, the project's instructions take precedence.

## Context

### Terminology:

- Say "project" instead of "account" — a project contains an AWS account and settings for sharing with other collaborators
- Say "team member" instead of "IAM user" — users are invited by email, not created or federated in IAM
- Say "AWS Settings" when referring to management tasks at [settings.aws.com](https://settings.aws.com/) (project management, billing, team members, spend limits). Users view their actual AWS resources in the AWS Management Console.
- Say "selected Region" when referring to the user's Region — not "home Region"
- The user has a managed IAM experience. This includes a managed service control policies (SCP) and resource control policies (RCP) that govern the use of AWS. They will still need to use IAM to create policies to let services work with each other. If there are questions about the SCPs or RCPs, go to the documentation at https://docs.aws.amazon.com/accounts/latest/reference/scps-and-rcps-for-projects.html

### Constraints:

- All projects share a single AWS Region determined by the user's contact address. Resources cannot be created in other Regions
- When developing:
  - MUST create all Regional resources in the project's assigned Region
  - You CAN create AWS WAF and Cloudwatch Logs resources in us-east-1 when there are global resources (like a global WAF instance) that require a connection to dependencies in us-east-1. You should not use these for any other reason, because resources in the selected Region will provide lower cost (due to no cross-Region traffic), increased availability (due to no cross-Region traffic), and easier manageability (due to not needing to look in another Region). When you need to do an inventory of resources, you need to look in both the selected Region and us-east-1 for Cloudwatch Logs or WAF resources.
  - MUST NOT attempt to create Lambda, API Gateway, or other Regional resources in any other Region
  - MUST direct users to confirm their Region in AWS Settings > View all projects > Overview > Additional Info > Region. If the user cannot confirm their Region, check in ~/.aws/config
  - MUST NOT use Lambda@Edge — excluded from both Lambda and CloudFront
  - MUST NOT use CloudFormation StackSets — no multi-account or multi-Region deployments
  - MUST NOT attempt cross-Region actions — no cross-Region replication for DynamoDB/S3/RDS, no multi-Region KMS keys
  - MUST NOT use Route 53 cross-Region routing — geolocation, latency-based, and failover routing policies are not available
  - CloudFront is a global service and its actions ARE allowed in `us-east-1`. A user can create a CloudFront distribution pointing to their project-region Lambda function URL or API Gateway. However, Lambda and API Gateway themselves MUST NOT be created in `us-east-1` — they must be in the project Region.
  - Reduced availability in `eu-north-1` specifically: Amazon Rekognition, Amazon Textract, Amazon Personalize, AWS App Runner are not available in that Region.
- IAM permissions for human access are managed by AWS. Don't assign roles to team members unless absolutely necessary
- The user may have a spend limit if they are on the paid plan. The limit that pauses their project if it's exceeded. If resources suddenly become inaccessible, ask if they have a spend limit configured. Only project owners can modify a spend limit.
- When developing:
  - MUST ask about spend limit status if the user reports sudden "Access Denied" errors on operations that previously worked
  - MUST direct users to check spend status in AWS Settings > Billing
  - MUST check if a user has upgraded their account to the paid plan
  - MUST ask the user if they want to clean up the successfully created resources or keep them to reduce cost
- The user sets up billing, creates spend limits, and retrieves and pays invoices in AWS Settings. The user creates budgets and optimizes their costs in the AWS Billing and Cost Management console
- Not all AWS services are available. If a service isn't working, do the following:
  1. Run the command `aws freetier get-account-plan-state`
  2. If accountPlanType": "FREE", check the [Free Tier supported services list](https://docs.aws.amazon.com/accounts/latest/reference/supported-services-sign-up-new.html#supported-services-free-tier) next,
  3. If accountPlanType": "PAID", check the [Paid Tier supported services list](https://docs.aws.amazon.com/accounts/latest/reference/supported-services-sign-up-new.html#supported-services-paid-plan).
  4. If neither list shows the service, check the [Not supported for this experience list](https://docs.aws.amazon.com/accounts/latest/reference/supported-services-sign-up-new.html#unsupported-services). The user will need to activate advanced features to access this service.
- Users can activate advanced AWS services and capabilities for their account.
- Before starting a task, check whether a relevant AWS skill is available. Load the skill with retrieve_skill and prefer its guidance over general knowledge.

### Help level

- help_level (required): LOW, MEDIUM, or HIGH. While a user is building, you MUST ask the user: "How much guidance would you like from me? Low (I only flag security risks), medium (I ask a couple of clarifying questions if something seems off), or high (I explain what I'm doing, suggest alternatives, and flag best practices)."

You CAN update this rule file to save a user's help_level.

Constraints for each level:

**LOW:**

- MUST follow all constraints in this context file
- MUST execute the user’s request without modification
- MUST NOT ask clarifying questions unless the action would create a security vulnerability
- MUST NOT suggest alternatives or improvements

**MEDIUM:**

- MUST execute the user's request
- MAY ask up to two clarifying questions per task if the request has an ambiguity or a potential issue
- MUST NOT repeat a question or suggestion the user has already dismissed
- MUST NOT explain trade-offs or alternatives unless the user asks

**HIGH:**

- MUST explain what each step does and why before executing it
- MUST suggest alternatives when a better approach exists
- MUST flag best practices and explain trade-offs
- MUST still execute the user's choice if they disagree with a suggestion

AWS help_level: HIGH (selected by the user).
<!-- END AWS Agent Toolkit rules -->
