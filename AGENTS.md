# Repository Guidelines

## Project Structure & Module Organization

VideoHelper combines a Go backend (`cmd/videohelper/`, `internal/`) with a Python model service (`ml/app/`) and a React frontend (`web/`). Tests live beside Go packages and in `ml/tests/`. Database migrations are in `migrations/`; configuration and the shared ML contract are in `config/` and `contracts/`. Keep the Chinese architecture documents, `docs/design.md` and `docs/spec.md`, aligned with contract changes. Runtime media belongs in object storage; local temporary files use `.local/`.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox for validation, demos, and scratch work. Unless the user explicitly requests work there, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from searches, builds, tests, and refactors. General repository work does not authorize access.

## Build, Test, and Development Commands

- `make deps-up` / `make deps-down`: start/stop local Postgres, Redis, and MinIO; preserve volumes.
- `make build`: build `bin/videohelper`; see `README.md` for local processing and search.
- `make test-go` / `make vet`: scoped Go race tests and static checks.
- `make fmt` / `make generate`: format Go and regenerate package-local sqlc outputs.
- `cd ml && uv run pytest` / `uv run ruff check .`: Python tests and lint.
- `npm --prefix web ci` / `npm --prefix web run dev` / `npm --prefix web run build`: install, serve on 5173, or type-check and build `web/dist`.
- `make test-web`: frontend helper tests (Node 22+).

## Coding Style & Testing Guidelines

Use `gofmt`, Python four-space indentation, and Ruff. Keep functions focused and reuse existing helpers. SQL uses singular snake_case names; wire fields use snake_case, IDs are strings, and intervals are half-open integer milliseconds. Do not hand-edit `internal/*/*db/` generated code.

Name Go tests `Test...` in `_test.go`, and Python tests `test_...`. Add meaningful checks for non-trivial behavior. Integration tests require explicit `VH_TEST_*` variables and disposable databases. No numerical coverage target is set. Fake-model tests do not establish real-model retrieval quality.

## Commit & Pull Request Guidelines

History uses short imperative messages, sometimes `feat:` or `test:` prefixes. Commit small verified units. PRs should explain behavior, scope, relevant issues, and checks actually run.

## Agent Development Workflow

- Start each new implementation on a new feature branch in a new, isolated worktree. Keep `main` for reviewed changes.
- Enable Ponytail FULL for every execution, including sub-agents: make minimal complete changes while preserving validation, cancellation, and data safety.
- Use `gpt-6.1-sol` at high effort for implementation sub-agents. Give independent parallel tasks separate branches and worktrees based on the feature branch.
- Work in small, verifiable units and commit frequently. Merge each completed and checked unit into the feature branch promptly.
- Update affected documentation, especially `docs/design.md` and `docs/spec.md`, alongside implementation. Avoid development journals and redundant acceptance reports.
- Review and verify the combined feature before merging it into `main`. Then remove the worktrees, merged task branches, and temporary services or artifacts created by this work; preserve unrelated changes, user data, and useful validation evidence.

## Current Implementation Boundary

Local text search uses WeMM-Embedding-2B for video, text, and image encoding, ASR for `speech`, and CLAP HTSAT unfused for the `audio` sound channel. Preserve source audio for ASR and CLAP. Pass query text unchanged; English translation is deferred. The `web/` frontend is React, TypeScript and Tailwind on Vite, one HTML entry per page; style with the theme tokens in `web/src/index.css`. LLM/Agent and AWS acceptance remain deferred.

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
