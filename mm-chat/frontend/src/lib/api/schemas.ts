import { z } from "zod";
import {
  API_INPUT_LIMITS,
  ATTACHMENT_LIMITS,
  CHAT_CONFIG_LIMITS,
  PLUGIN_EXECUTION_LIMITS,
  getAttachmentsPayloadChars,
} from "../../config/limits";
import { getRemoteAttachmentUrlError } from "../security/remoteAttachment";
import { getPluginExecutionArgsError } from "../plugin/execution";
import { BYOK_ALG } from "../byok/shared";

const Base64UrlStringSchema = z.string().regex(/^[A-Za-z0-9_-]+$/);

export const EncryptedSecretEnvelopeSchema = z.object({
  v: z.literal(1),
  kid: z.string().min(1).max(128),
  alg: z.literal(BYOK_ALG),
  iv: Base64UrlStringSchema.min(1).max(1_024),
  wrappedKey: Base64UrlStringSchema.min(1).max(16_384),
  ciphertext: Base64UrlStringSchema.min(1).max(65_536),
  context: z.string().min(1).max(200),
});

function rejectPlainSecretField(
  value: unknown,
  ctx: z.RefinementCtx,
  path: string[],
  label: string,
) {
  if (typeof value === "string" && value.trim()) {
    ctx.addIssue({
      code: "custom",
      path,
      message: `${label} must be sent as an encrypted BYOK secret`,
    });
  }
}

function omitPlainSecretField<
  T extends Record<string, unknown>,
  K extends string,
>(value: T, field: K): Omit<T, K> {
  const next = { ...value };
  delete next[field];
  return next as Omit<T, K>;
}

export const ProviderRuntimeConfigSchema = z
  .object({
    type: z.enum(["OpenAI", "Gemini", "OpenAI Compatible", "Anthropic"]),
    source: z.literal("server-default").optional(),
    apiKey: z.unknown().optional(),
    apiKeySecret: EncryptedSecretEnvelopeSchema.optional(),
    baseUrl: z.string().max(2_048).optional(),
    name: z.string().max(120).optional(),
  })
  .strict()
  .superRefine((provider, ctx) => {
    rejectPlainSecretField(
      provider.apiKey,
      ctx,
      ["apiKey"],
      "Provider API key",
    );
  })
  .transform((provider) => omitPlainSecretField(provider, "apiKey"));

const JsonLikeSchema: z.ZodType<unknown> = z.lazy(() =>
  z.union([
    z.string(),
    z.number(),
    z.boolean(),
    z.null(),
    z.array(JsonLikeSchema),
    z.record(z.string(), JsonLikeSchema),
  ]),
);

export const ModelNameSchema = z
  .string()
  .min(1)
  .max(API_INPUT_LIMITS.maxModelNameChars);

export const AttachmentSchema = z.object({
  id: z.string().default(""),
  mimeType: z.string().min(1).max(ATTACHMENT_LIMITS.maxMimeTypeChars),
  data: z.string().max(ATTACHMENT_LIMITS.maxBase64Chars).optional(),
  url: z.string().max(ATTACHMENT_LIMITS.maxUrlChars).optional(),
  fileName: z.string().min(1).max(ATTACHMENT_LIMITS.maxFileNameChars),
});

export const ToolCallSchema = z
  .object({
    id: z.string().min(1),
    name: z.string().min(1).max(128),
    args: z.record(z.string(), JsonLikeSchema).default({}),
    status: z
      .enum(["pending", "running", "success", "error", "skipped"])
      .optional(),
    result: JsonLikeSchema.optional(),
    isError: z.boolean().optional(),
  })
  .transform((toolCall) => ({
    ...toolCall,
    status:
      toolCall.status ||
      (toolCall.isError
        ? "error"
        : toolCall.result !== undefined
          ? "success"
          : "pending"),
  }));

export const SkillInvocationSchema = z
  .object({
    id: z
      .string()
      .min(1)
      .max(160)
      .regex(/^[A-Za-z0-9_-]+$/),
    title: z.string().min(1).max(160),
    description: z.string().max(2_048).optional(),
    category: z.string().min(1).max(120),
    mode: z.enum(["manual", "auto"]),
  })
  .strict();

export const MessageSchema = z.object({
  id: z.string().default(""),
  role: z.enum(["user", "model"]),
  content: z.string().max(2_000_000).default(""),
  reasoning: z.string().max(2_000_000).optional(),
  timestamp: z.number().default(0),
  attachments: z.array(AttachmentSchema).max(20).optional(),
  toolCalls: z
    .array(ToolCallSchema)
    .max(PLUGIN_EXECUTION_LIMITS.maxStreamedToolCalls)
    .optional(),
  skillInvocations: z.array(SkillInvocationSchema).max(20).optional(),
  model: ModelNameSchema.optional(),
});

const FunctionParametersSchema = z
  .object({
    type: z.string().optional(),
    properties: z.record(z.string(), JsonLikeSchema).optional(),
    required: z.array(z.string()).optional(),
  })
  .passthrough();

const ToolSchema = z
  .object({
    type: z.literal("function"),
    function: z
      .object({
        name: z
          .string()
          .min(1)
          .max(128)
          .regex(/^[A-Za-z0-9_-]+$/),
        description: z.string().max(2_048).optional(),
        parameters: FunctionParametersSchema.optional(),
      })
      .strict(),
  })
  .strict();

export const ChatRequestSchema = z
  .object({
    provider: ProviderRuntimeConfigSchema,
    modelName: ModelNameSchema,
    history: z.array(MessageSchema).max(400),
    newMessage: z.string().max(API_INPUT_LIMITS.maxChatTextChars),
    attachments: z
      .array(AttachmentSchema)
      .max(ATTACHMENT_LIMITS.maxCount)
      .optional(),
    config: z
      .object({
        temperature: z
          .number()
          .min(CHAT_CONFIG_LIMITS.minTemperature)
          .max(CHAT_CONFIG_LIMITS.maxTemperature)
          .optional(),
        useReasoning: z.boolean().optional(),
        reasoningEffort: z
          .enum(["auto", "low", "medium", "high", "xhigh", "max"])
          .optional(),
        useSearch: z.boolean().optional(),
        searchMode: z.enum(["off", "model_builtin", "external"]).optional(),
      })
      .strict()
      .optional(),
    systemInstruction: z
      .string()
      .max(API_INPUT_LIMITS.maxSystemInstructionChars)
      .optional(),
    tools: z.array(ToolSchema).max(64).optional(),
  })
  .strict()
  .superRefine((request, ctx) => {
    const totalPayloadChars = getAttachmentsPayloadChars(
      request.attachments || [],
    );

    if (totalPayloadChars > ATTACHMENT_LIMITS.maxTotalBase64Chars) {
      ctx.addIssue({
        code: "custom",
        path: ["attachments"],
        message: "Attachment payload is too large",
      });
    }

    request.attachments?.forEach((attachment, index) => {
      if (!attachment.url) return;

      const remoteUrlError = getRemoteAttachmentUrlError(attachment.url);
      if (remoteUrlError) {
        ctx.addIssue({
          code: "custom",
          path: ["attachments", index, "url"],
          message: remoteUrlError,
        });
      }
    });
  });

export const SimpleGenerateRequestSchema = z
  .object({
    provider: ProviderRuntimeConfigSchema,
    modelName: ModelNameSchema,
    prompt: z.string().min(1).max(API_INPUT_LIMITS.maxSimplePromptChars),
  })
  .strict();

export const AuxiliaryGenerateRequestSchema = z
  .object({
    provider: ProviderRuntimeConfigSchema,
    modelName: ModelNameSchema,
    history: z.array(MessageSchema).max(100).optional(),
    userMessage: z
      .string()
      .max(API_INPUT_LIMITS.maxAuxiliaryTextChars)
      .optional(),
  })
  .strict();

const PluginFunctionSchema = z
  .object({
    name: z
      .string()
      .min(1)
      .max(128)
      .regex(/^[A-Za-z0-9_-]+$/),
    description: z.string().max(2_048).optional(),
    parameters: FunctionParametersSchema.optional(),
    path: z.string().min(1).max(1_024),
    method: z
      .enum(["GET", "POST", "PUT", "PATCH", "DELETE"])
      .or(z.enum(["get", "post", "put", "patch", "delete"])),
  })
  .strict();

const PluginSchema = z
  .object({
    id: z.string().min(1).max(200),
    title: z.string().max(300).optional(),
    description: z.string().max(5_000).optional(),
    logoUrl: z.string().max(2_048).optional(),
    manifestUrl: z.string().max(2_048).optional(),
    externalDocsUrl: z.string().max(2_048).optional(),
    baseUrl: z.string().max(2_048).optional(),
    category: z.string().max(120).optional(),
    categories: z.array(z.string().max(120)).max(20).optional(),
    added: z.string().max(120).optional(),
    functions: z.array(PluginFunctionSchema).max(40).optional(),
    builtIn: z.boolean().optional(),
    auth: z
      .object({
        type: z.enum(["bearer", "apiKey", "basic", "oauth2", "none"]),
        name: z.string().max(120).optional(),
        in: z.enum(["header", "query"]).optional(),
        required: z.boolean().optional(),
      })
      .optional(),
  })
  .strict();

const PluginAuthConfigSchema = z
  .object({
    type: z.enum(["bearer", "apiKey", "none", "oauth2"]).optional(),
    value: z.unknown().optional(),
    valueSecret: EncryptedSecretEnvelopeSchema.optional(),
    key: z.string().max(120).optional(),
    addTo: z.enum(["header", "query"]).optional(),
  })
  .strict()
  .superRefine((authConfig, ctx) => {
    rejectPlainSecretField(
      authConfig.value,
      ctx,
      ["value"],
      "Plugin auth value",
    );
  })
  .transform((authConfig) => omitPlainSecretField(authConfig, "value"))
  .optional();

export const ToolExecutionSchema = z
  .object({
    plugin: PluginSchema,
    functionDef: PluginFunctionSchema,
    args: z.record(z.string(), JsonLikeSchema).default({}),
    authConfig: PluginAuthConfigSchema,
  })
  .strict()
  .superRefine((request, ctx) => {
    const argsError = getPluginExecutionArgsError(request.args);
    if (argsError) {
      ctx.addIssue({
        code: "custom",
        path: ["args"],
        message: argsError,
      });
    }
  });

export const PluginExecutionRequestSchema = z
  .object({
    pluginId: z.string().min(1).max(200),
    functionName: z.string().min(1).max(128),
    args: z.record(z.string(), JsonLikeSchema).default({}),
    authConfig: PluginAuthConfigSchema,
    callId: z.string().max(200).optional(),
  })
  .strict()
  .superRefine((request, ctx) => {
    const argsError = getPluginExecutionArgsError(request.args);
    if (argsError) {
      ctx.addIssue({
        code: "custom",
        path: ["args"],
        message: argsError,
      });
    }
  });

export const PluginInstallSchema = z
  .object({
    plugin: PluginSchema.partial().optional(),
    customInput: z.string().max(2_000_000).optional(),
  })
  .strict();

export const ImageGenerateRequestSchema = z
  .object({
    provider: ProviderRuntimeConfigSchema,
    modelName: ModelNameSchema,
    prompt: z.string().min(1).max(8_000),
  })
  .strict();

export const VoiceSynthesizeRequestSchema = z
  .object({
    text: z.string().min(1).max(10_000),
    provider: z.enum(["default", "elevenlabs", "mimo", "browser", "model"]),
    apiKey: z.unknown().optional(),
    apiKeySecret: EncryptedSecretEnvelopeSchema.optional(),
    voiceId: z.string().max(120).optional(),
    modelId: z.string().max(120).optional(),
    modelProvider: ProviderRuntimeConfigSchema.optional(),
  })
  .strict()
  .superRefine((request, ctx) => {
    rejectPlainSecretField(request.apiKey, ctx, ["apiKey"], "Voice API key");
  })
  .superRefine((request, ctx) => {
    if (request.provider === "elevenlabs" && !request.voiceId?.trim()) {
      ctx.addIssue({
        code: "custom",
        path: ["voiceId"],
        message: "ElevenLabs voice ID is required",
      });
    }
  })
  .transform((request) => omitPlainSecretField(request, "apiKey"));

export const VoiceTranscribeRequestSchema = z
  .object({
    provider: z.enum(["default", "elevenlabs", "mimo", "browser", "model"]),
    apiKey: z.unknown().optional(),
    apiKeySecret: EncryptedSecretEnvelopeSchema.optional(),
    modelId: z.string().max(120).optional(),
    modelProvider: ProviderRuntimeConfigSchema.optional(),
    language: z.enum(["auto", "en", "zh", "ja"]).optional(),
  })
  .strict()
  .superRefine((request, ctx) => {
    rejectPlainSecretField(request.apiKey, ctx, ["apiKey"], "Voice API key");
  })
  .transform((request) => omitPlainSecretField(request, "apiKey"));
