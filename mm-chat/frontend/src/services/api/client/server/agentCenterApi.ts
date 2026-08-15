import { z } from "zod";

import { ApiClientError } from "../errors";
import type { AgentCenterApi } from "../types";
import type { HttpClient } from "./httpClient";

const string = z.string().min(1);
const count = z.number().int().nonnegative();
const positive = z.number().int().positive();
const fingerprint = z.string().regex(/^sha256:[a-f0-9]{64}$/);

const packageVersionSchema = z
  .object({
    packageFingerprint: fingerprint,
    runtimeBundleFingerprint: fingerprint.optional(),
    sbomFingerprint: fingerprint,
    name: string,
    version: string,
    description: z.string(),
    license: z.string().optional(),
    compatibility: z.string().optional(),
    allowedTools: z.array(z.string()),
    capabilityRequests: z.array(
      z
        .object({
          capability: string,
          actions: z.array(z.string()),
          reason: z.string(),
        })
        .strict(),
    ),
    hasRuntime: z.boolean(),
    fileCount: count,
    packageBytes: count,
    expandedBytes: count,
    createdAt: string,
  })
  .strict();

const packageCandidateSchema = z
  .object({
    id: string,
    sourceType: string,
    sourceRef: string,
    sourceArtifactSha256: fingerprint,
    package: packageVersionSchema,
    status: string,
    admissionEligible: z.boolean(),
    validationSummary: z.string(),
    reviewReason: z.string().optional(),
    revision: positive,
    createdAt: string,
    updatedAt: string,
  })
  .strict();

const installationSchema = z
  .object({
    id: string,
    admissionId: string,
    packageFingerprint: fingerprint,
    name: string,
    version: string,
    description: z.string(),
    allowedTools: z.array(z.string()),
    revision: positive,
    createdAt: string,
    updatedAt: string,
  })
  .strict();

const shadowPolicySchema = z
  .object({
    revision: count,
    enabled: z.boolean(),
    mode: z.enum(["synthetic", "read_only"]),
    admissionId: z.string().optional(),
    packageFingerprint: fingerprint.optional(),
    runtimeBundleFingerprint: fingerprint.optional(),
    cohortBasisPoints: count,
    maxObservations: positive,
    maxErrors: count,
    startsAt: z.string().optional(),
    expiresAt: z.string().optional(),
    updatedAt: z.string(),
  })
  .strict();

const shadowSnapshotSchema = z
  .object({
    policy: shadowPolicySchema,
    optIn: z
      .object({
        optedIn: z.boolean(),
        generation: count,
        policyRevision: count,
        updatedAt: z.string(),
      })
      .strict(),
    cohortSelected: z.boolean(),
    eligible: z.boolean(),
    effective: z.boolean(),
    heldReasonCode: z.string(),
    observationCount: count,
    errorCount: count,
    productCanary: z
      .object({
        activationId: z.string().optional(),
        planFingerprint: fingerprint.optional(),
        remainingRequests: count,
      })
      .strict(),
  })
  .strict();

const statusSchema = z
  .object({
    isAdministrator: z.boolean(),
    runtime: z
      .object({
        state: string,
        reasonCode: string,
        executable: z.boolean(),
        productCanary: z.boolean(),
        scheduler: z.boolean(),
        learningWorker: z.boolean(),
      })
      .strict(),
    shadow: shadowSnapshotSchema,
  })
  .strict();

const runSummarySchema = z
  .object({
    id: string,
    state: string,
    snapshotFingerprint: fingerprint,
    requestFingerprint: fingerprint,
    createdAt: string,
    updatedAt: string,
    terminalAt: z.string().optional(),
    cancellationState: z.string().optional(),
    cancellationMode: z.string().optional(),
    cancellationReason: z.string().optional(),
  })
  .strict();

const productCanaryRequestSchema = z
  .object({
    id: z.string().regex(/^product_request_[a-z0-9]{16,64}$/),
    activationId: z.string().regex(/^activation_[a-z0-9]{16,64}$/),
    state: z.enum(["queued", "claimed", "completed", "failed"]),
    policyRevision: positive,
    optGeneration: positive,
    requestFingerprint: fingerprint,
    failureCount: count,
    errorCode: z.string().optional(),
    createdAt: string,
    updatedAt: string,
    terminalAt: z.string().optional(),
  })
  .strict();

const approvalSchema = z
  .object({
    intentId: string,
    intentFingerprint: fingerprint,
    toolIdentity: string,
    capability: string,
    action: string,
    argumentsFingerprint: fingerprint,
    approvalClass: string,
    approvalRevision: count,
    state: string,
    expiresAt: string,
    approvedAt: z.string().optional(),
    terminalAt: z.string().optional(),
    errorCode: z.string().optional(),
  })
  .strict();

const runDetailSchema = z
  .object({
    run: runSummarySchema,
    steps: z.array(
      z
        .object({
          id: string,
          ordinal: count,
          kind: string,
          state: string,
          currentGeneration: count,
          createdAt: string,
          updatedAt: string,
          terminalAt: z.string().optional(),
        })
        .strict(),
    ),
    attempts: z.array(
      z
        .object({
          id: string,
          stepId: string,
          generation: positive,
          state: string,
          leaseExpiresAt: string,
          createdAt: string,
          updatedAt: string,
          terminalAt: z.string().optional(),
        })
        .strict(),
    ),
    events: z.array(
      z
        .object({
          id: string,
          stepId: z.string().optional(),
          attemptId: z.string().optional(),
          sequence: positive,
          occurredAt: string,
          kind: string,
          entity: string,
          from: z.string().optional(),
          to: string,
          actorType: string,
          reasonCode: string,
          generation: count.optional(),
          leaseExpiresAt: z.string().optional(),
        })
        .strict(),
    ),
    approvals: z.array(approvalSchema),
    children: z.array(
      z
        .object({
          runId: string,
          parentRunId: z.string().optional(),
          depth: count,
          state: string,
          packageFingerprint: fingerprint,
          runtimeBundleFingerprint: fingerprint,
          grantFingerprint: fingerprint,
          registryFingerprint: fingerprint,
          expiresAt: string,
          createdAt: string,
        })
        .strict(),
    ),
    artifacts: z.array(
      z
        .object({
          id: string,
          attemptId: string,
          generation: positive,
          name: string,
          mediaType: string,
          size: count,
          fingerprint,
          createdAt: string,
          downloadUrl: string,
        })
        .strict(),
    ),
  })
  .strict();

const scheduleSummarySchema = z
  .object({
    id: string,
    state: string,
    currentRevision: positive,
    revisionFingerprint: fingerprint,
    nextTriggerAt: z.string().optional(),
    scheduleExpression: string,
    timezone: string,
    calculator: string,
    automationClass: string,
    approvalId: string,
    packageFingerprint: fingerprint,
    runtimeBundleFingerprint: fingerprint,
    missedPolicy: string,
    overlapPolicy: string,
    createdAt: string,
    updatedAt: string,
  })
  .strict();

const scheduleDetailSchema = z
  .object({
    id: string,
    currentRevision: positive,
    revisionFingerprint: fingerprint,
    state: string,
    nextTriggerAt: z.string().optional(),
    createdAt: string,
    updatedAt: string,
    spec: z.record(z.string(), z.unknown()),
  })
  .strict();

const checkSchema = z
  .object({
    id: string,
    kind: string,
    status: string,
    reasonCode: string,
    suiteFingerprint: fingerprint,
    evidenceFingerprint: fingerprint,
    durationMillis: count,
    metrics: z.record(z.string(), z.number()),
  })
  .strict();

const draftSchema = z
  .object({
    id: string,
    ownerUserId: string,
    state: string,
    revision: positive,
    draftFingerprint: fingerprint,
    basePackageFingerprint: fingerprint,
    proposedPackageFingerprint: fingerprint,
    runtimeBundleFingerprint: fingerprint,
    name: string,
    version: string,
    checkGeneration: count,
    checkAttempts: count,
    admissionId: z.string().optional(),
    createdAt: string,
    updatedAt: string,
    objectDeletedAt: z.string().optional(),
    checks: z.array(checkSchema),
  })
  .strict();

const diffSchema = z
  .object({
    path: string,
    change: string,
    beforeFingerprint: z.string(),
    afterFingerprint: z.string(),
    beforeSize: count,
    afterSize: count,
    beforeText: z.string(),
    afterText: z.string(),
    binary: z.boolean(),
  })
  .strict();

function parse<T>(schema: z.ZodType<T>, value: unknown, subject: string): T {
  const result = schema.safeParse(value);
  if (!result.success) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      `Server returned an invalid Agent Center ${subject}.`,
    );
  }
  return result.data;
}

export function createServerAgentCenterApiShell(
  httpClient: HttpClient,
): AgentCenterApi {
  return {
    async getStatus(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/agent-center/status",
        { signal: options.signal },
      );
      return parse(statusSchema, response, "status");
    },

    async listPackageStore(input = {}) {
      const query = new URLSearchParams({
        page: String(input.page ?? 1),
        pageSize: String(input.pageSize ?? 20),
      });
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store?${query}`,
        { signal: input.signal },
      );
      return parse(
        z
          .object({
            items: z.array(packageCandidateSchema),
            page: positive,
            pageSize: positive,
            totalCount: count,
            totalPages: count,
          })
          .strict(),
        response,
        "Package Skill Store",
      );
    },

    async getPackageSkill(candidateId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store/items/${encodeURIComponent(candidateId)}`,
        { signal: options.signal },
      );
      const envelope = parse(
        z.object({ skill: packageCandidateSchema }).strict(),
        response,
        "Package Skill detail",
      );
      return envelope.skill;
    },

    async listPackageLibrary(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/skills/library",
        { signal: options.signal },
      );
      return parse(
        z.object({ skills: z.array(installationSchema) }).strict(),
        response,
        "Package Skill library",
      ).skills;
    },

    async installPackageSkill(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store/items/${encodeURIComponent(input.candidateId)}/install`,
        {
          method: "POST",
          body: { packageFingerprint: input.packageFingerprint },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ skill: installationSchema }).strict(),
        response,
        "Package Skill install",
      ).skill;
    },

    async uninstallPackageSkill(input) {
      await httpClient.requestJson<void>(
        `/v1/skills/library/${encodeURIComponent(input.installationId)}?revision=${encodeURIComponent(String(input.revision))}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async listRuns(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/runs?limit=${encodeURIComponent(String(options.limit ?? 50))}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ runs: z.array(runSummarySchema) }).strict(),
        response,
        "Run list",
      ).runs;
    },

    async getRun(runId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/runs/${encodeURIComponent(runId)}`,
        { signal: options.signal },
      );
      return parse(runDetailSchema, response, "Run detail");
    },

    async downloadArtifact(input) {
      return httpClient.requestBinary(
        `/v1/agent-center/runs/${encodeURIComponent(input.runId)}/artifacts/${encodeURIComponent(input.artifactId)}/content`,
        { signal: input.signal },
      );
    },

    async enqueueRun(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/agent-center/runs",
        {
          method: "POST",
          body: {
            expectedPolicyRevision: input.expectedPolicyRevision,
            expectedGeneration: input.expectedGeneration,
          },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ request: productCanaryRequestSchema }).strict(),
        response,
        "product canary request",
      ).request;
    },

    async cancelRun(input) {
      await httpClient.requestJson<unknown>(
        `/v1/agent-center/runs/${encodeURIComponent(input.runId)}/cancel`,
        {
          method: "POST",
          body: {
            expectedState: input.expectedState,
            snapshotFingerprint: input.snapshotFingerprint,
            mode: input.mode,
            reasonCode: input.reasonCode,
          },
          signal: input.signal,
        },
      );
    },

    async decideApproval(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/runs/${encodeURIComponent(input.runId)}/approvals/${encodeURIComponent(input.intentId)}/decision`,
        {
          method: "POST",
          body: {
            intentFingerprint: input.intentFingerprint,
            decision: input.decision,
            reasonCode: input.reasonCode,
            expectedRevision: input.expectedRevision,
          },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ approval: approvalSchema }).strict(),
        response,
        "approval",
      ).approval;
    },

    async listSchedules(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/schedules?limit=${encodeURIComponent(String(options.limit ?? 50))}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ schedules: z.array(scheduleSummarySchema) }).strict(),
        response,
        "Schedule list",
      ).schedules;
    },

    async getSchedule(scheduleId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/schedules/${encodeURIComponent(scheduleId)}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ schedule: scheduleDetailSchema }).strict(),
        response,
        "Schedule detail",
      ).schedule;
    },

    async createSchedule(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/agent-center/schedules",
        {
          method: "POST",
          body: {
            templateId: input.templateId,
            expectedRevision: input.expectedRevision,
            spec: input.spec,
            reasonCode: input.reasonCode,
            ...(input.activateAt ? { activateAt: input.activateAt } : {}),
          },
          signal: input.signal,
        },
      );
      return parse(
        z
          .object({ schedule: scheduleDetailSchema, created: z.boolean() })
          .strict(),
        response,
        "Schedule create",
      ).schedule;
    },

    async changeScheduleLifecycle(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/schedules/${encodeURIComponent(input.scheduleId)}/lifecycle`,
        {
          method: "POST",
          body: {
            expectedRevision: input.expectedRevision,
            state: input.state,
            reasonCode: input.reasonCode,
          },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ schedule: scheduleDetailSchema }).strict(),
        response,
        "Schedule lifecycle",
      ).schedule;
    },

    async listDrafts(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/learning/drafts?limit=${encodeURIComponent(String(options.limit ?? 50))}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ drafts: z.array(draftSchema) }).strict(),
        response,
        "Draft list",
      ).drafts;
    },

    async getDraft(draftId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/learning/drafts/${encodeURIComponent(draftId)}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ draft: draftSchema }).strict(),
        response,
        "Draft detail",
      ).draft;
    },

    async getDraftDiff(draftId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/agent-center/learning/drafts/${encodeURIComponent(draftId)}/diff`,
        { signal: options.signal },
      );
      return parse(
        z.object({ files: z.array(diffSchema) }).strict(),
        response,
        "Draft diff",
      ).files;
    },

    async reviewDraft(input) {
      await httpClient.requestJson<unknown>(
        `/v1/agent-center/learning/drafts/${encodeURIComponent(input.draftId)}/review`,
        {
          method: "POST",
          body: {
            decision: input.decision,
            expectedRevision: input.expectedRevision,
            draftFingerprint: input.draftFingerprint,
            proposedPackageFingerprint: input.proposedPackageFingerprint,
            reasonCode: input.reasonCode,
          },
          signal: input.signal,
        },
      );
    },

    async setShadowOptIn(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/agent-center/shadow/opt-in",
        {
          method: "PUT",
          body: {
            expectedGeneration: input.expectedGeneration,
            policyRevision: input.policyRevision,
            optedIn: input.optedIn,
            reasonCode: input.reasonCode,
          },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ shadow: shadowSnapshotSchema }).strict(),
        response,
        "Shadow opt-in",
      ).shadow;
    },
  };
}
