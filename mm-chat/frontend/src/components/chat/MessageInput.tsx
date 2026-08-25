"use client";
import React, {
  useState,
  useRef,
  useEffect,
  useMemo,
  useCallback,
  useImperativeHandle,
  forwardRef,
  useId,
} from "react";
import { v7 as uuidv7 } from "uuid";
import {
  SendHorizontal,
  Paperclip,
  Mic,
  X,
  StopCircle,
  Loader2,
  Cpu,
  Globe,
  Lightbulb,
  Link,
  ChevronDown,
  FileUp,
  ImageUp,
  Square,
  Library,
  PencilSparkles,
  Check,
  Bot,
  MessageCircle,
  ShieldCheck,
  ShieldAlert,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type {
  Attachment,
  ChatToolMode,
  AgentPermissionMode,
  ReasoningEffort,
  SearchMode,
} from "@/types";
import type { ModelInfo } from "@/services/api/chatService";
import { createNeoChatApiClient } from "@/services/api/client";
import Tooltip from "../ui/Tooltip";
import { Dialog } from "@/components/ui/primitives";
import RemoteFileModal from "../modals/RemoteFileModal";
import KnowledgeSelectionModal from "../knowledge/KnowledgeSelectionModal";
import MessageInputAttachmentTray from "./MessageInputAttachmentTray";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useChatStore } from "@/store/core/chatStore";
import { useCoreSettingsStore } from "@/store/core/coreSettingsStore";
import { getTaskModel, useSettingsStore } from "@/store/core/settingsStore";
import {
  transcribeAudio,
  startBrowserSpeechRecognition,
} from "@/services/api/voiceService";
import {
  ATTACHMENT_LIMITS,
  formatBytes,
  getAttachmentPayloadChars,
  getAttachmentsPayloadChars,
} from "@/config/limits";
import { parseModelString } from "@/lib/utils/model";
import { stopMediaStreamTracks } from "@/lib/utils/mediaRecording";
import { logDevError } from "@/lib/utils/devLogger";
import {
  extractChatAttachmentFilesFromClipboard,
  extractChatAttachmentFilesFromDrop,
  getChatAttachmentFileSelectionMessage,
  selectChatAttachmentFiles,
} from "@/lib/utils/chatAttachmentFiles";
import {
  getSearchCompatibility,
  getSearchProviderLabel,
  type SearchCompatibilityReason,
} from "@/lib/settings/search";
import {
  isKnowledgeAttachment,
  MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS,
  normalizeKnowledgeCollectionIds,
} from "@/lib/utils/knowledgeAttachments";
import { polishTextContent } from "@/services/artifactService";
import {
  formatRecordingTime as formatTime,
  isNativeMediaFile,
  shouldSubmitOnEnter,
  truncateMiddle,
} from "@/lib/utils/messageInputHelpers";
import {
  getReasoningEffortOptions,
  isReasoningEffort,
} from "@/lib/chat/reasoning";
import { getModelBuiltInSearchAvailability } from "@/lib/chat/searchCapabilities";
import {
  filterSlashCommands,
  slashCommandInsertion,
  type SlashCommandDefinition,
} from "@/lib/chat/slashCommands";

type MessageInputVariant = "default" | "hero";

type OpenComposerSection =
  "tool-mode" | "permission" | "reasoning" | "search" | null;

interface MessageInputProps {
  onSend: (
    text: string,
    attachments: Attachment[],
  ) => boolean | void | Promise<boolean | void>;
  onStop?: () => void;
  disabled: boolean;
  availableModels?: ModelInfo[];
  selectedModel?: string;
  onSelectModel?: (model: string) => void;
  searchMode?: SearchMode;
  onSearchModeChange?: (mode: SearchMode) => void;
  isReasoningEnabled?: boolean;
  reasoningEffort?: ReasoningEffort;
  onReasoningChange?: (enabled: boolean, effort: ReasoningEffort) => void;
  localSessionToolsDisabled?: boolean;
  allowSearchWhenSessionToolsDisabled?: boolean;
  allowReasoningWhenSessionToolsDisabled?: boolean;
  toolMode: ChatToolMode;
  effectiveToolMode: ChatToolMode;
  canSelectAgentMode: boolean;
  onToolModeChange: (mode: ChatToolMode) => void;
  permissionMode?: AgentPermissionMode;
  availablePermissionModes?: readonly AgentPermissionMode[];
  showPermissionControl?: boolean;
  permissionLocked?: boolean;
  onPermissionModeChange?: (
    mode: AgentPermissionMode,
    fullAccessAcknowledged: boolean,
  ) => void;
  onLocalSessionToolUnavailable?: (action: string) => void;
  knowledgeCollectionIds?: readonly string[];
  onKnowledgeCollectionIdsChange?: (
    collectionIds: string[],
  ) => void | Promise<void>;
  slashCommands?: readonly SlashCommandDefinition[];
  slashCommandsLoading?: boolean;
  variant?: MessageInputVariant;
}

export interface MessageInputRef {
  setValue: (value: string) => void;
  focus: () => void;
  setAttachments: (attachments: Attachment[]) => void;
}

const logInputError = logDevError;

const iconButtonFocusClass =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-background";

const iconButtonBaseClass =
  "inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg";

const reasoningEffortItemClass =
  "data-[state=checked]:bg-violet-100 data-[state=checked]:font-medium data-[state=checked]:text-violet-700 dark:data-[state=checked]:bg-violet-900/40 dark:data-[state=checked]:text-violet-200";

const searchModeItemClass =
  "data-[state=checked]:bg-blue-100 data-[state=checked]:font-medium data-[state=checked]:text-blue-700 dark:data-[state=checked]:bg-blue-900/40 dark:data-[state=checked]:text-blue-200";

const toolModeItemClass = "h-auto items-start py-2";

const toolModeItemTextClass = "flex min-w-0 flex-1 flex-col";

const toolModeDescriptionClass =
  "whitespace-normal break-words text-xs leading-4 font-normal text-muted-foreground";

const loadChatService = () => import("@/services/api/chatService");
const EMPTY_KNOWLEDGE_COLLECTION_IDS: readonly string[] = [];

const MessageInput = forwardRef<MessageInputRef, MessageInputProps>(
  (
    {
      onSend,
      onStop,
      disabled,
      availableModels = [],
      selectedModel = "",
      onSelectModel,
      searchMode = "off",
      onSearchModeChange,
      isReasoningEnabled,
      reasoningEffort,
      onReasoningChange,
      localSessionToolsDisabled = false,
      allowSearchWhenSessionToolsDisabled = false,
      allowReasoningWhenSessionToolsDisabled = false,
      toolMode,
      effectiveToolMode,
      canSelectAgentMode,
      onToolModeChange,
      permissionMode = "workspace-write",
      availablePermissionModes = [],
      showPermissionControl = false,
      permissionLocked = false,
      onPermissionModeChange,
      onLocalSessionToolUnavailable,
      knowledgeCollectionIds = EMPTY_KNOWLEDGE_COLLECTION_IDS,
      onKnowledgeCollectionIdsChange,
      slashCommands = [],
      slashCommandsLoading = false,
      variant = "default",
    },
    ref,
  ) => {
    const [input, setInput] = useState("");
    const [attachments, setAttachments] = useState<Attachment[]>([]);
    const [isRecording, setIsRecording] = useState(false);
    const [isTranscribing, setIsTranscribing] = useState(false);
    const [recordingSeconds, setRecordingSeconds] = useState(0);
    const [showModelSelect, setShowModelSelect] = useState(false);
    const [showAttachMenu, setShowAttachMenu] = useState(false);
    const [openComposerSection, setOpenComposerSection] =
      useState<OpenComposerSection>(null);
    const [showRemoteModal, setShowRemoteModal] = useState(false);
    const [showKBModal, setShowKBModal] = useState(false);
    const [showFullAccessConfirmation, setShowFullAccessConfirmation] =
      useState(false);
    const [errorMsg, setErrorMsg] = useState<string | null>(null);
    const [isDragUploadActive, setIsDragUploadActive] = useState(false);
    const [isPolishingInput, setIsPolishingInput] = useState(false);
    const [isParsingAttachments, setIsParsingAttachments] = useState(false);
    const [isSavingKnowledgeSelection, setIsSavingKnowledgeSelection] =
      useState(false);
    const [knowledgeCollectionNames, setKnowledgeCollectionNames] = useState<
      Record<string, string>
    >({});
    const [slashSelection, setSlashSelection] = useState(0);
    const [slashMenuDismissed, setSlashMenuDismissed] = useState(false);

    const handleComposerSectionOpenChange = useCallback(
      (section: Exclude<OpenComposerSection, null>, open: boolean) => {
        setOpenComposerSection((current) =>
          open ? section : current === section ? null : current,
        );
      },
      [],
    );

    const t = useTranslations("MessageInput");
    const { chatConfig, setChatConfig } = useChatStore();
    const {
      modelMetadata,
      customModelMetadata,
      voice,
      updateVoiceSettings,
      search,
    } = useSettingsStore();
    const providers = useCoreSettingsStore((state) => state.providers);

    const knowledgeApiClient = useMemo(() => createNeoChatApiClient(), []);

    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const imageInputRef = useRef<HTMLInputElement>(null);
    const textFallbackInputRef = useRef<HTMLInputElement>(null);
    const messageInputId = useId();
    const errorMessageId = useId();
    const attachFileInputId = useId();
    const attachImageInputId = useId();
    const attachTextFallbackInputId = useId();
    const isHeroVariant = variant === "hero";
    const effectiveUseReasoning = isReasoningEnabled ?? chatConfig.useReasoning;
    const effectiveReasoningEffort =
      reasoningEffort ?? chatConfig.reasoningEffort;
    const conversationKnowledgeEnabled =
      typeof onKnowledgeCollectionIdsChange === "function";
    const normalizedKnowledgeCollectionIds = useMemo(
      () =>
        normalizeKnowledgeCollectionIds(
          knowledgeCollectionIds.filter(
            (value): value is string => typeof value === "string",
          ),
        ).slice(0, MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS),
      [knowledgeCollectionIds],
    );
    const knowledgeCollectionIdsKey =
      normalizedKnowledgeCollectionIds.join(",");

    useEffect(() => {
      if (!conversationKnowledgeEnabled || !knowledgeCollectionIdsKey) {
        setKnowledgeCollectionNames({});
        return;
      }

      const controller = new AbortController();
      const collectionIds = knowledgeCollectionIdsKey.split(",");
      void Promise.allSettled(
        collectionIds.map((collectionId) =>
          knowledgeApiClient.knowledge.getCollection({
            collectionId,
            signal: controller.signal,
          }),
        ),
      ).then((results) => {
        if (controller.signal.aborted) return;
        const names: Record<string, string> = {};
        results.forEach((result, index) => {
          if (result.status === "fulfilled") {
            names[collectionIds[index]] = result.value.name;
          }
        });
        setKnowledgeCollectionNames(names);
      });

      return () => controller.abort();
    }, [
      conversationKnowledgeEnabled,
      knowledgeApiClient,
      knowledgeCollectionIdsKey,
    ]);

    // Browser Speech Rec
    const recognitionRef = useRef<any>(null);
    // MediaRecorder Audio Capture
    const mediaRecorderRef = useRef<MediaRecorder | null>(null);
    const mediaStreamRef = useRef<MediaStream | null>(null);
    const audioChunksRef = useRef<Blob[]>([]);
    const recordingKindRef = useRef<"browser" | "media" | null>(null);

    const timerRef = useRef<any>(null);
    const isMountedRef = useRef(true);
    const recordingSessionRef = useRef(0);
    const fileSelectionRunRef = useRef(0);
    const polishRunRef = useRef(0);
    const dragDepthRef = useRef(0);
    const draftRevisionRef = useRef(0);
    const submissionRunRef = useRef(0);
    const isSubmittingRef = useRef(false);

    const clearRecordingTimer = useCallback(() => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    }, []);

    const releaseMediaStream = useCallback(
      (stream = mediaStreamRef.current) => {
        stopMediaStreamTracks(stream);
        if (!stream || mediaStreamRef.current === stream) {
          mediaStreamRef.current = null;
        }
      },
      [],
    );

    useEffect(() => {
      isMountedRef.current = true;

      return () => {
        isMountedRef.current = false;
        recordingSessionRef.current += 1;
        fileSelectionRunRef.current += 1;
        polishRunRef.current += 1;
        clearRecordingTimer();

        if (recognitionRef.current) {
          try {
            recognitionRef.current.stop();
          } catch {
            // The browser may throw if recognition has already ended.
          }
          recognitionRef.current = null;
        }

        const recorder = mediaRecorderRef.current;
        if (recorder) {
          recorder.ondataavailable = null;
          recorder.onstop = null;
          if (recorder.state !== "inactive") {
            try {
              recorder.stop();
            } catch {
              // Ignore stale recorder state during component teardown.
            }
          }
          mediaRecorderRef.current = null;
        }

        audioChunksRef.current = [];
        recordingKindRef.current = null;
        releaseMediaStream();
      };
    }, [clearRecordingTimer, releaseMediaStream]);

    const appendAttachments = (incoming: Attachment[]) => {
      if (incoming.length === 0) return;

      const accepted: Attachment[] = [];
      let totalPayloadChars = getAttachmentsPayloadChars(attachments);
      let rejectedByCount = 0;
      let rejectedBySize = 0;

      for (const attachment of incoming) {
        if (
          attachments.length + accepted.length >=
          ATTACHMENT_LIMITS.maxCount
        ) {
          rejectedByCount += 1;
          continue;
        }

        const payloadChars = getAttachmentPayloadChars(attachment);
        if (
          totalPayloadChars + payloadChars >
          ATTACHMENT_LIMITS.maxTotalBase64Chars
        ) {
          rejectedBySize += 1;
          continue;
        }

        totalPayloadChars += payloadChars;
        accepted.push(attachment);
      }

      if (rejectedByCount > 0) {
        setErrorMsg(
          t("attachmentLimitReached", { max: ATTACHMENT_LIMITS.maxCount }),
        );
      } else if (rejectedBySize > 0) {
        setErrorMsg(
          t("attachmentsExceedSize", {
            size: formatBytes(ATTACHMENT_LIMITS.maxTotalBase64Chars),
          }),
        );
      }

      if (accepted.length > 0) {
        draftRevisionRef.current += 1;
        setAttachments((prev) => [...prev, ...accepted]);
      }
    };

    useImperativeHandle(ref, () => ({
      setValue: (value: string) => {
        draftRevisionRef.current += 1;
        setInput(value);
        requestAnimationFrame(() => {
          if (textareaRef.current) {
            textareaRef.current.style.height = "auto";
            textareaRef.current.style.height =
              textareaRef.current.scrollHeight + "px";
          }
        });
      },
      focus: () => {
        textareaRef.current?.focus();
      },
      setAttachments: (atts: Attachment[]) => {
        draftRevisionRef.current += 1;
        setAttachments(atts);
      },
    }));

    // Clear error after 3 seconds
    useEffect(() => {
      if (errorMsg) {
        const timer = setTimeout(() => setErrorMsg(null), 3000);
        return () => clearTimeout(timer);
      }
    }, [errorMsg]);

    useEffect(() => {
      const handleEscape = (e: KeyboardEvent) => {
        if (e.key !== "Escape") return;
        setShowAttachMenu(false);
        setShowModelSelect(false);
      };

      document.addEventListener("keydown", handleEscape);
      return () => document.removeEventListener("keydown", handleEscape);
    }, []);

    const searchCompatibility = useMemo(() => {
      return getSearchCompatibility({
        searchProvider: search.provider,
        searchConfig: search.configs.default,
      });
    }, [search]);

    const selectedModelIdentity = useMemo(
      () => parseModelString(selectedModel),
      [selectedModel],
    );
    const selectedProvider = useMemo(
      () =>
        selectedModelIdentity.providerId
          ? providers.find(
              (provider) => provider.id === selectedModelIdentity.providerId,
            )
          : undefined,
      [providers, selectedModelIdentity.providerId],
    );
    const modelBuiltInSearch = useMemo(
      () =>
        getModelBuiltInSearchAvailability({
          provider: selectedProvider,
          modelId: selectedModelIdentity.modelName,
        }),
      [selectedModelIdentity.modelName, selectedProvider],
    );

    const getSearchUnavailableMessage = (
      reason: SearchCompatibilityReason | undefined,
    ) => {
      switch (reason) {
        case "server_search_unavailable":
          return t("searchUnavailableGeneric");
        default:
          return t("searchUnavailableGeneric");
      }
    };

    const searchModeLabel = t("searchModeExternal", {
      provider: getSearchProviderLabel(searchCompatibility.provider),
    });
    const modelBuiltInSearchLabel =
      modelBuiltInSearch.protocol === "gemini_google_search"
        ? t("searchModeGeminiGoogle")
        : modelBuiltInSearch.protocol === "anthropic_web_search"
          ? t("searchModeAnthropicWeb")
          : modelBuiltInSearch.protocol === "openai_responses"
            ? t("searchModeOpenAIWeb")
            : t("searchModeModelBuiltIn");
    const modelBuiltInUnavailableMessage =
      modelBuiltInSearch.reason === "provider_unavailable"
        ? t("searchUnavailableNoProvider")
        : modelBuiltInSearch.reason === "admin_test_required"
          ? t("searchUnavailableAdminTest")
          : t("searchUnavailableModelBuiltIn");

    const isSearchEnabled = searchMode !== "off";
    const searchTooltip =
      searchMode === "external"
        ? searchCompatibility.enabled
          ? searchModeLabel
          : getSearchUnavailableMessage(searchCompatibility.reason)
        : searchMode === "model_builtin"
          ? modelBuiltInSearch.enabled
            ? modelBuiltInSearchLabel
            : modelBuiltInUnavailableMessage
          : t("searchModeOff");

    const notifyLocalSessionToolUnavailable = useCallback(
      (action: string) => {
        onLocalSessionToolUnavailable?.(action);
      },
      [onLocalSessionToolUnavailable],
    );

    const handleSearchModeChange = (value: string) => {
      if (value !== "off" && value !== "model_builtin" && value !== "external")
        return;
      if (
        value === "external" &&
        localSessionToolsDisabled &&
        !allowSearchWhenSessionToolsDisabled
      ) {
        notifyLocalSessionToolUnavailable("search toggle");
        return;
      }
      if (value === "external" && !searchCompatibility.enabled) {
        setErrorMsg(getSearchUnavailableMessage(searchCompatibility.reason));
        return;
      }
      if (value === "model_builtin" && !modelBuiltInSearch.enabled) {
        setErrorMsg(modelBuiltInUnavailableMessage);
        return;
      }
      onSearchModeChange?.(value);
    };

    // Group models by provider name
    const groupedModels = useMemo(() => {
      const groups: Record<string, ModelInfo[]> = {};
      availableModels.forEach((model) => {
        const pName = model.providerName || "System";
        if (!groups[pName]) groups[pName] = [];
        groups[pName].push(model);
      });
      return groups;
    }, [availableModels]);

    // --- Capabilities Resolution ---
    const modelCapabilities = useMemo(() => {
      if (!selectedModel)
        return {
          vision: false,
          attachment: false,
          audio: false,
          reasoning: false,
        };

      const { modelName: modelId } = parseModelString(selectedModel);
      // Prioritize custom metadata overrides
      const meta = customModelMetadata[modelId] || modelMetadata[modelId];

      return {
        vision: meta?.modalities?.input?.includes("image") ?? false,
        attachment: meta?.attachment ?? false,
        audio: meta?.modalities?.input?.includes("audio") ?? false,
        reasoning: meta?.reasoning ?? false,
      };
    }, [selectedModel, modelMetadata, customModelMetadata]);

    const isReasoningSupported = useMemo(() => {
      if (!selectedModel) return false;
      const { modelName: modelId } = parseModelString(selectedModel);

      // Check custom metadata first, then global metadata
      const meta = customModelMetadata[modelId] || modelMetadata[modelId];

      if (meta && meta.reasoning !== undefined) return meta.reasoning;

      // Fallback to name heuristic
      const lower = modelId.toLowerCase();
      return (
        lower.includes("thinking") ||
        lower.includes("reasoner") ||
        lower.includes("gpt-5") ||
        lower.includes("o1") ||
        lower.includes("r1")
      );
    }, [selectedModel, modelMetadata, customModelMetadata]);

    const reasoningEffortOptions = useMemo(() => {
      if (!selectedModel) return getReasoningEffortOptions("");
      return getReasoningEffortOptions(
        parseModelString(selectedModel).modelName,
      );
    }, [selectedModel]);

    const reasoningEffortLabel = (effort: ReasoningEffort): string => {
      switch (effort) {
        case "auto":
          return t("reasoningEffortAuto");
        case "low":
          return t("reasoningEffortLow");
        case "medium":
          return t("reasoningEffortMedium");
        case "high":
          return t("reasoningEffortHigh");
        case "xhigh":
          return t("reasoningEffortXHigh");
        case "max":
          return t("reasoningEffortMax");
      }
    };

    const updateReasoningSelection = (selection: string) => {
      if (
        localSessionToolsDisabled &&
        !allowReasoningWhenSessionToolsDisabled
      ) {
        notifyLocalSessionToolUnavailable("reasoning effort");
        return;
      }
      if (selection === "off") {
        if (onReasoningChange) {
          onReasoningChange(false, effectiveReasoningEffort);
        } else {
          setChatConfig({ useReasoning: false });
        }
        return;
      }
      if (!isReasoningEffort(selection)) return;
      if (onReasoningChange) {
        onReasoningChange(true, selection);
      } else {
        setChatConfig({ useReasoning: true, reasoningEffort: selection });
      }
    };

    const filteredSlashCommands = useMemo(
      () => filterSlashCommands(slashCommands, input),
      [input, slashCommands],
    );
    const slashQueryActive =
      input.trimStart().startsWith("/") && !input.includes("\n");
    const slashMenuOpen = !slashMenuDismissed && slashQueryActive;

    useEffect(() => {
      setSlashSelection((current) =>
        Math.min(current, Math.max(filteredSlashCommands.length - 1, 0)),
      );
    }, [filteredSlashCommands.length]);

    const selectSlashCommand = useCallback(
      (command: SlashCommandDefinition) => {
        draftRevisionRef.current += 1;
        setInput(slashCommandInsertion(command));
        setSlashMenuDismissed(true);
        requestAnimationFrame(() => textareaRef.current?.focus());
      },
      [],
    );

    const handleKeyDown = (e: React.KeyboardEvent) => {
      if (slashMenuOpen && filteredSlashCommands.length > 0) {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          const direction = e.key === "ArrowDown" ? 1 : -1;
          setSlashSelection((current) => {
            const count = filteredSlashCommands.length;
            return (current + direction + count) % count;
          });
          return;
        }
        if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
          e.preventDefault();
          selectSlashCommand(filteredSlashCommands[slashSelection]);
          return;
        }
        if (e.key === "Escape") {
          e.preventDefault();
          setSlashMenuDismissed(true);
          return;
        }
      }
      if (
        shouldSubmitOnEnter({
          key: e.key,
          shiftKey: e.shiftKey,
          isComposing: e.nativeEvent.isComposing,
        })
      ) {
        e.preventDefault();
        void handleSend();
      }
    };

    const handleSend = async () => {
      if (
        (!input.trim() && attachments.length === 0) ||
        disabled ||
        isParsingAttachments ||
        !selectedModel ||
        isSubmittingRef.current
      ) {
        return;
      }

      const submittedInput = input;
      const submittedAttachments = attachments;
      const draftRevision = draftRevisionRef.current;
      const submissionRun = submissionRunRef.current + 1;
      submissionRunRef.current = submissionRun;
      isSubmittingRef.current = true;
      setInput("");
      setAttachments([]);
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
      }

      const restoreSubmittedDraft = () => {
        if (
          submissionRunRef.current !== submissionRun ||
          draftRevisionRef.current !== draftRevision
        ) {
          return;
        }
        setInput(submittedInput);
        setAttachments(submittedAttachments);
        requestAnimationFrame(() => {
          if (!textareaRef.current) return;
          textareaRef.current.style.height = "auto";
          textareaRef.current.style.height =
            textareaRef.current.scrollHeight + "px";
        });
      };

      try {
        const accepted = await onSend(submittedInput, submittedAttachments);
        if (accepted === false) {
          restoreSubmittedDraft();
        }
      } catch (error) {
        logInputError("Failed to send message", error);
        restoreSubmittedDraft();
      } finally {
        if (submissionRunRef.current === submissionRun) {
          isSubmittingRef.current = false;
        }
      }
    };

    const handlePolishInput = async () => {
      const originalText = input;
      if (
        !originalText.trim() ||
        disabled ||
        isTranscribing ||
        isParsingAttachments ||
        isPolishingInput
      ) {
        return;
      }

      const runId = polishRunRef.current + 1;
      polishRunRef.current = runId;
      setIsPolishingInput(true);
      setErrorMsg(null);

      try {
        let replacement = "";
        const { streamGenerateContent } = await loadChatService();
        await streamGenerateContent(
          getTaskModel("promptOptimization"),
          polishTextContent(originalText),
          (text) => {
            if (!isMountedRef.current || polishRunRef.current !== runId) return;
            replacement = text;
            setInput(text);
          },
        );

        if (!isMountedRef.current || polishRunRef.current !== runId) return;
        if (!replacement.trim()) {
          setInput(originalText);
          setErrorMsg(t("polishFailed"));
        }
      } catch (error) {
        logInputError("Failed to polish input text", error);
        if (isMountedRef.current && polishRunRef.current === runId) {
          setInput(originalText);
          setErrorMsg(t("polishFailed"));
        }
      } finally {
        if (isMountedRef.current && polishRunRef.current === runId) {
          setIsPolishingInput(false);
        }
      }
    };

    // Convert Blob to Base64 String (helper)
    const blobToBase64 = (blob: Blob): Promise<string> => {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onloadend = () => {
          if (typeof reader.result === "string") {
            resolve(reader.result.split(",")[1]); // remove data:audio/webm;base64, prefix
          } else {
            reject(new Error("Failed to convert blob"));
          }
        };
        reader.onerror = reject;
        reader.readAsDataURL(blob);
      });
    };

    const startRecording = async () => {
      setErrorMsg(null);
      if (voice.autoTranscribe && voice.sttProvider === "browser") {
        const sessionId = recordingSessionRef.current + 1;
        recordingSessionRef.current = sessionId;
        try {
          recognitionRef.current = startBrowserSpeechRecognition(
            voice.sttLanguage,
            {
              onTranscript: (text) => {
                if (
                  !isMountedRef.current ||
                  recordingSessionRef.current !== sessionId
                ) {
                  return;
                }
                setInput((prev) => prev + (prev ? " " : "") + text);
              },
              onError: (err) => {
                if (
                  !isMountedRef.current ||
                  recordingSessionRef.current !== sessionId
                ) {
                  return;
                }
                logInputError("Speech recognition error", err);
                stopRecording();
              },
              onEnd: () => {
                if (
                  !isMountedRef.current ||
                  recordingSessionRef.current !== sessionId
                ) {
                  return;
                }
                recordingSessionRef.current += 1;
                recognitionRef.current = null;
                recordingKindRef.current = null;
                clearRecordingTimer();
                setIsRecording(false);
              },
            },
          );

          recordingKindRef.current = "browser";
          setIsRecording(true);
          setRecordingSeconds(0);
          clearRecordingTimer();
          timerRef.current = setInterval(() => {
            setRecordingSeconds((prev) => prev + 1);
          }, 1000);
        } catch (e) {
          logInputError("Failed to start browser recording", e);
          recognitionRef.current = null;
          recordingKindRef.current = null;
          if (
            isMountedRef.current &&
            recordingSessionRef.current === sessionId
          ) {
            setErrorMsg(
              e instanceof Error ? e.message : t("failedToStartRecognition"),
            );
          }
        }
      } else {
        const sessionId = recordingSessionRef.current + 1;
        recordingSessionRef.current = sessionId;
        let stream: MediaStream | null = null;
        try {
          stream = await navigator.mediaDevices.getUserMedia({
            audio: true,
          });
          if (
            !isMountedRef.current ||
            recordingSessionRef.current !== sessionId
          ) {
            releaseMediaStream(stream);
            return;
          }
          mediaStreamRef.current = stream;

          let mimeType = "audio/webm";
          if (!MediaRecorder.isTypeSupported("audio/webm")) {
            if (MediaRecorder.isTypeSupported("audio/mp4")) {
              mimeType = "audio/mp4";
            } else {
              mimeType = ""; // Let browser decide default
            }
          }

          const mediaRecorder = mimeType
            ? new MediaRecorder(stream, { mimeType })
            : new MediaRecorder(stream);
          mediaRecorderRef.current = mediaRecorder;
          audioChunksRef.current = [];

          mediaRecorder.ondataavailable = (event) => {
            if (recordingSessionRef.current !== sessionId) return;
            if (event.data.size > 0) {
              audioChunksRef.current.push(event.data);
            }
          };

          mediaRecorder.onstop = async () => {
            const recordedType = mediaRecorder.mimeType || "audio/webm";
            const audioChunks = audioChunksRef.current;
            audioChunksRef.current = [];
            releaseMediaStream(stream);
            if (mediaRecorderRef.current === mediaRecorder) {
              mediaRecorderRef.current = null;
            }
            if (recordingKindRef.current === "media") {
              recordingKindRef.current = null;
            }
            clearRecordingTimer();

            if (
              !isMountedRef.current ||
              recordingSessionRef.current !== sessionId
            ) {
              return;
            }
            setIsRecording(false);

            const audioBlob = new Blob(audioChunks, {
              type: recordedType,
            });

            if (voice.autoTranscribe) {
              setIsTranscribing(true);
              try {
                const text = await transcribeAudio(audioBlob, voice);
                if (
                  text &&
                  isMountedRef.current &&
                  recordingSessionRef.current === sessionId
                ) {
                  setInput((prev) => prev + (prev ? " " : "") + text);
                }
              } catch (e) {
                logInputError("Transcription failed", e);
                if (
                  isMountedRef.current &&
                  recordingSessionRef.current === sessionId
                ) {
                  setErrorMsg(
                    e instanceof Error ? e.message : t("transcriptionFailed"),
                  );
                }
              } finally {
                if (
                  isMountedRef.current &&
                  recordingSessionRef.current === sessionId
                ) {
                  setIsTranscribing(false);
                }
              }
            } else {
              try {
                const base64Data = await blobToBase64(audioBlob);
                if (
                  !isMountedRef.current ||
                  recordingSessionRef.current !== sessionId
                ) {
                  return;
                }
                let extension = "webm";
                if (recordedType.includes("mp4")) extension = "mp4";
                else if (recordedType.includes("aac")) extension = "aac";
                else if (recordedType.includes("ogg")) extension = "ogg";
                else if (recordedType.includes("wav")) extension = "wav";

                const newAtt: Attachment = {
                  id: uuidv7(),
                  mimeType: recordedType,
                  data: base64Data,
                  fileName: `Voice Note ${new Date().toLocaleTimeString().replace(/:/g, "-")}.${extension}`,
                };
                appendAttachments([newAtt]);
              } catch (e) {
                logInputError("Failed to process audio attachment", e);
                if (
                  isMountedRef.current &&
                  recordingSessionRef.current === sessionId
                ) {
                  setErrorMsg(t("failedToProcessAudio"));
                }
              }
            }
          };

          mediaRecorder.start();
          recordingKindRef.current = "media";
          setIsRecording(true);
          setRecordingSeconds(0);
          clearRecordingTimer();
          timerRef.current = setInterval(() => {
            setRecordingSeconds((prev) => prev + 1);
          }, 1000);
        } catch (e) {
          logInputError("Failed to access microphone", e);
          releaseMediaStream(stream);
          mediaRecorderRef.current = null;
          recordingKindRef.current = null;
          if (
            isMountedRef.current &&
            recordingSessionRef.current === sessionId
          ) {
            setErrorMsg(t("failedToAccessMicrophone"));
          }
        }
      }
    };

    const stopRecording = () => {
      if (recordingKindRef.current === "browser") {
        recordingSessionRef.current += 1;
        if (recognitionRef.current) {
          try {
            recognitionRef.current.stop();
          } catch {
            // Recognition can already be inactive by the time the UI stops it.
          }
          recognitionRef.current = null;
        }
      } else if (recordingKindRef.current === "media") {
        if (
          mediaRecorderRef.current &&
          mediaRecorderRef.current.state !== "inactive"
        ) {
          try {
            mediaRecorderRef.current.stop();
          } catch {
            releaseMediaStream();
            mediaRecorderRef.current = null;
          }
        } else {
          releaseMediaStream();
          mediaRecorderRef.current = null;
        }
      }
      recordingKindRef.current = null;
      if (isMountedRef.current) {
        setIsRecording(false);
      }
      clearRecordingTimer();
    };

    const toggleRecording = () => {
      if (isRecording) {
        stopRecording();
      } else {
        startRecording();
      }
    };

    const fileToBase64 = (file: File): Promise<string> => {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.readAsDataURL(file);
        reader.onload = () => resolve(reader.result as string);
        reader.onerror = (error) => reject(error);
      });
    };

    const canAttachFileNatively = (file: File): boolean => {
      if (!isNativeMediaFile(file)) return false;
      if (modelCapabilities.attachment) return true;
      if (file.type.startsWith("image/")) return modelCapabilities.vision;
      if (file.type.startsWith("audio/")) return modelCapabilities.audio;
      return false;
    };

    const processSelectedFiles = async (
      files: File[],
      {
        documentsOnly = false,
        closeAttachMenu = false,
      }: { documentsOnly?: boolean; closeAttachMenu?: boolean } = {},
    ) => {
      if (files.length === 0) return;

      const runId = fileSelectionRunRef.current + 1;
      fileSelectionRunRef.current = runId;
      const selection = selectChatAttachmentFiles(attachments.length, files);
      const selectionMessage = getChatAttachmentFileSelectionMessage(selection);
      if (selectionMessage) setErrorMsg(selectionMessage);
      const newAttachments: Attachment[] = [];

      setIsParsingAttachments(true);
      try {
        for (const file of selection.accepted) {
          const useRawAttachment =
            knowledgeApiClient.mode === "server" ||
            (!documentsOnly && canAttachFileNatively(file));
          try {
            if (useRawAttachment) {
              const base64 = await fileToBase64(file);
              if (
                !isMountedRef.current ||
                fileSelectionRunRef.current !== runId
              ) {
                return;
              }
              const base64Data = base64.split(",")[1];

              newAttachments.push({
                id: uuidv7(),
                mimeType: file.type || "application/octet-stream",
                data: base64Data,
                fileName: file.name,
              });
              continue;
            }
            throw new Error(
              "Document parsing requires the server-backed file pipeline.",
            );
          } catch (err) {
            if (
              !isMountedRef.current ||
              fileSelectionRunRef.current !== runId
            ) {
              return;
            }
            logInputError(
              useRawAttachment
                ? "Error reading file"
                : "Error parsing document attachment",
              err,
            );
            setErrorMsg(
              t(
                useRawAttachment ? "failedToReadFile" : "failedToParseDocument",
                { fileName: file.name },
              ),
            );
          }
        }

        if (isMountedRef.current && fileSelectionRunRef.current === runId) {
          appendAttachments(newAttachments);
          if (closeAttachMenu) setShowAttachMenu(false);
        }
      } finally {
        if (isMountedRef.current && fileSelectionRunRef.current === runId) {
          setIsParsingAttachments(false);
        }
      }
    };

    const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
      const inputEl = e.currentTarget;
      if (inputEl.files && inputEl.files.length > 0) {
        await processSelectedFiles(Array.from(inputEl.files) as File[], {
          closeAttachMenu: true,
        });
        if (inputEl.value) inputEl.value = "";
      }
    };

    const handleTextFallbackSelect = async (
      e: React.ChangeEvent<HTMLInputElement>,
    ) => {
      const inputEl = e.currentTarget;
      if (inputEl.files && inputEl.files.length > 0) {
        await processSelectedFiles(Array.from(inputEl.files) as File[], {
          documentsOnly: true,
        });
        if (inputEl.value) inputEl.value = "";
      }
    };

    const removeAttachment = (id: string) => {
      draftRevisionRef.current += 1;
      setAttachments((prev) => prev.filter((a) => a.id !== id));
    };

    const persistKnowledgeCollectionIds = async (collectionIds: string[]) => {
      if (!onKnowledgeCollectionIdsChange) return;
      const normalizedIds = normalizeKnowledgeCollectionIds(
        collectionIds,
      ).slice(0, MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS);
      setIsSavingKnowledgeSelection(true);
      try {
        await onKnowledgeCollectionIdsChange(normalizedIds);
      } catch (error) {
        setErrorMsg(t("knowledgeSelectionFailed"));
        throw error;
      } finally {
        setIsSavingKnowledgeSelection(false);
      }
    };

    // Adjust textarea height
    useEffect(() => {
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
        textareaRef.current.style.height =
          textareaRef.current.scrollHeight + "px";
      }
    }, [input]);

    const currentModelName =
      availableModels.find((m) => m.name === selectedModel)?.displayName ||
      selectedModel ||
      t("noModelSelected");
    const hasKnowledgeAttachments = attachments.some(isKnowledgeAttachment);
    const isInputBusy = disabled || isTranscribing || isParsingAttachments;
    const textareaMinHeightClass = isHeroVariant
      ? "min-h-[5em]"
      : "min-h-[2em]";
    const composerPaddingClass = isHeroVariant ? "mb-0 md:mb-18" : "";

    const eventHasFiles = (types: DOMStringList | readonly string[]) =>
      Array.from(types).includes("Files");

    const handleComposerDragEnter = (e: React.DragEvent<HTMLDivElement>) => {
      if (isInputBusy || !eventHasFiles(e.dataTransfer.types)) return;
      e.preventDefault();
      e.stopPropagation();
      dragDepthRef.current += 1;
      setIsDragUploadActive(true);
    };

    const handleComposerDragOver = (e: React.DragEvent<HTMLDivElement>) => {
      if (isInputBusy || !eventHasFiles(e.dataTransfer.types)) return;
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = "copy";
      setIsDragUploadActive(true);
    };

    const handleComposerDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
      if (!eventHasFiles(e.dataTransfer.types)) return;
      e.preventDefault();
      e.stopPropagation();
      dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
      if (dragDepthRef.current === 0) {
        setIsDragUploadActive(false);
      }
    };

    const handleComposerDrop = (e: React.DragEvent<HTMLDivElement>) => {
      if (isInputBusy) return;
      const files = extractChatAttachmentFilesFromDrop(e.dataTransfer);
      if (files.length === 0) return;
      e.preventDefault();
      e.stopPropagation();
      dragDepthRef.current = 0;
      setIsDragUploadActive(false);
      void processSelectedFiles(files);
    };

    const handleComposerPaste = (
      e: React.ClipboardEvent<HTMLTextAreaElement>,
    ) => {
      if (isInputBusy) return;
      const files = extractChatAttachmentFilesFromClipboard(e.clipboardData);
      if (files.length === 0) return;
      e.preventDefault();
      void processSelectedFiles(files);
    };

    return (
      <div
        className={`glass-shell chat-composer-surface relative flex w-full flex-col rounded-xl border focus-within:ring-2 focus-within:ring-blue-100/50 dark:focus-within:ring-blue-900/30 focus-within:border-blue-400/50 transition-[background-color,border-color,box-shadow] duration-200 ${composerPaddingClass}`}
        aria-busy={isInputBusy}
        onDragEnter={handleComposerDragEnter}
        onDragOver={handleComposerDragOver}
        onDragLeave={handleComposerDragLeave}
        onDrop={handleComposerDrop}
      >
        {/* Modals */}
        {showRemoteModal && (
          <RemoteFileModal
            onClose={() => setShowRemoteModal(false)}
            onAttach={(att) => appendAttachments([att])}
            capabilities={modelCapabilities}
          />
        )}

        {showKBModal && conversationKnowledgeEnabled && (
          <KnowledgeSelectionModal
            onClose={() => setShowKBModal(false)}
            onSelectCollections={persistKnowledgeCollectionIds}
            initialSelectedCollectionIds={normalizedKnowledgeCollectionIds}
            maxSelectedCollections={MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS}
          />
        )}

        {showFullAccessConfirmation && (
          <Dialog
            open
            role="alertdialog"
            onClose={() => setShowFullAccessConfirmation(false)}
            title={t("fullAccessTitle")}
            className="z-10000 max-w-md rounded-2xl dark:bg-card"
          >
            <div className="p-5">
              <div className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-3 text-red-800 dark:border-red-900/60 dark:bg-red-950/25 dark:text-red-200">
                <ShieldAlert
                  size={20}
                  className="mt-0.5 shrink-0"
                  aria-hidden="true"
                />
                <p className="text-sm leading-6">
                  {t("fullAccessConfirmation")}
                </p>
              </div>
              <div className="mt-5 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setShowFullAccessConfirmation(false)}
                  className="rounded-xl px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 dark:text-muted-foreground dark:hover:bg-muted"
                >
                  {t("permissionCancel")}
                </button>
                <button
                  type="button"
                  disabled={permissionLocked || isInputBusy}
                  onClick={() => {
                    setShowFullAccessConfirmation(false);
                    onPermissionModeChange?.("danger-full-access", true);
                  }}
                  className="rounded-xl bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {t("enableFullAccess")}
                </button>
              </div>
            </div>
          </Dialog>
        )}

        {/* Error Message Toast */}
        {errorMsg && (
          <div
            id={errorMessageId}
            role="status"
            aria-live="polite"
            className="absolute -top-10 left-0 right-0 flex justify-center z-50 animate-in fade-in slide-in-from-bottom-2"
          >
            <div className="bg-red-600 text-white text-xs px-3 py-1.5 rounded-full shadow-lg font-medium flex items-center gap-2 dark:bg-red-500">
              <button
                type="button"
                aria-label={t("dismissError")}
                className={`rounded-full p-0.5 hover:bg-white/15 transition-colors ${iconButtonFocusClass}`}
                onClick={() => setErrorMsg(null)}
              >
                <X size={12} aria-hidden="true" />
              </button>
              <span>{errorMsg}</span>
            </div>
          </div>
        )}

        {!errorMsg && isParsingAttachments && (
          <div
            role="status"
            aria-live="polite"
            className="absolute -top-10 left-0 right-0 z-50 flex justify-center animate-in fade-in slide-in-from-bottom-2"
          >
            <div className="flex items-center gap-2 rounded-full bg-gray-900 px-3 py-1.5 text-xs font-medium text-white shadow-lg dark:bg-muted dark:text-foreground">
              <Loader2 size={12} className="animate-spin" aria-hidden="true" />
              <span>{t("parsingDocument")}</span>
            </div>
          </div>
        )}

        {isDragUploadActive && (
          <div
            className="absolute inset-1 z-40 flex flex-col items-center justify-center rounded-lg border border-dashed border-brand/60 bg-white/85 text-center shadow-sm backdrop-blur-md dark:bg-background/85"
            aria-hidden="true"
          >
            <FileUp size={20} className="mb-2 text-brand" />
            <div className="text-sm font-semibold text-foreground">
              {t("dropFilesTitle")}
            </div>
            <div className="mt-1 max-w-60 text-xs text-muted-foreground">
              {t("dropFilesHint")}
            </div>
          </div>
        )}

        {/* Attachments Preview Area */}
        <MessageInputAttachmentTray
          attachments={attachments}
          onRemove={removeAttachment}
          ariaLabel={t("attachedFiles")}
        />

        {conversationKnowledgeEnabled &&
          normalizedKnowledgeCollectionIds.length > 0 && (
            <div
              className="flex flex-wrap gap-1.5 px-3 pt-2"
              aria-label={t("selectedKnowledgeBases")}
            >
              {normalizedKnowledgeCollectionIds.map((collectionId) => {
                const name =
                  knowledgeCollectionNames[collectionId] || collectionId;
                return (
                  <span
                    key={collectionId}
                    className="inline-flex max-w-56 items-center gap-1 rounded-full border border-purple-200 bg-purple-50 px-2.5 py-1 text-xs font-medium text-purple-700 dark:border-purple-800/70 dark:bg-purple-950/35 dark:text-purple-200"
                  >
                    <Library size={12} aria-hidden="true" />
                    <span className="truncate">{name}</span>
                    <button
                      type="button"
                      aria-label={t("removeKnowledgeBase", { name })}
                      className={`rounded-full p-0.5 hover:bg-purple-200/70 dark:hover:bg-purple-800/60 ${iconButtonFocusClass}`}
                      disabled={isSavingKnowledgeSelection}
                      onClick={() => {
                        void persistKnowledgeCollectionIds(
                          normalizedKnowledgeCollectionIds.filter(
                            (id) => id !== collectionId,
                          ),
                        ).catch(() => undefined);
                      }}
                    >
                      <X size={11} aria-hidden="true" />
                    </button>
                  </span>
                );
              })}
            </div>
          )}

        {/* Text Input */}
        {slashMenuOpen && (
          <div
            role="listbox"
            aria-label={t("slashCommands")}
            className="absolute bottom-full left-0 right-0 z-50 mb-2 max-h-80 overflow-y-auto rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl dark:border-border dark:bg-card"
          >
            {slashCommandsLoading ? (
              <div className="px-3 py-4 text-sm text-muted-foreground">
                {t("slashCommandsLoading")}
              </div>
            ) : filteredSlashCommands.length === 0 ? (
              <div className="px-3 py-4 text-sm text-muted-foreground">
                {t("slashCommandsEmpty")}
              </div>
            ) : (
              filteredSlashCommands.map((command, index) => (
                <React.Fragment key={command.command}>
                  {index === 0 ||
                  filteredSlashCommands[index - 1].group !== command.group ? (
                    <div className="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                      {command.group}
                    </div>
                  ) : null}
                  <button
                    type="button"
                    role="option"
                    aria-selected={index === slashSelection}
                    className={`flex w-full items-start justify-between gap-3 rounded-lg px-3 py-2 text-left ${
                      index === slashSelection
                        ? "bg-cyan-50 text-cyan-950 dark:bg-cyan-950/40 dark:text-cyan-100"
                        : "hover:bg-slate-50 dark:hover:bg-muted"
                    }`}
                    onMouseDown={(event) => event.preventDefault()}
                    onMouseEnter={() => setSlashSelection(index)}
                    onClick={() => selectSlashCommand(command)}
                  >
                    <span className="min-w-0">
                      <span className="block font-mono text-sm font-semibold">
                        {command.command}
                        {command.argumentHint ? (
                          <span className="ml-2 font-sans font-normal text-muted-foreground">
                            {command.argumentHint}
                          </span>
                        ) : null}
                      </span>
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {command.description}
                      </span>
                    </span>
                  </button>
                </React.Fragment>
              ))
            )}
          </div>
        )}
        <label htmlFor={messageInputId} className="sr-only">
          {t("message")}
        </label>
        <textarea
          id={messageInputId}
          name="message"
          ref={textareaRef}
          className={`w-full px-4 pt-3 bg-transparent focus:outline-0 text-gray-800 dark:text-foreground placeholder-gray-500 dark:placeholder:text-muted-foreground resize-none max-h-32 md:max-h-48 text-(length:--neo-font-size-base) leading-5 ${textareaMinHeightClass} overflow-y-auto overscroll-contain custom-scrollbar`}
          placeholder={
            isRecording
              ? voice.sttProvider === "browser"
                ? t("listening")
                : t("recording")
              : t("askAnything")
          }
          autoComplete="off"
          aria-describedby={errorMsg ? errorMessageId : undefined}
          value={input}
          onChange={(e) => {
            draftRevisionRef.current += 1;
            setInput(e.target.value);
            setSlashMenuDismissed(false);
            setSlashSelection(0);
          }}
          onKeyDown={handleKeyDown}
          onPaste={handleComposerPaste}
          disabled={isInputBusy}
        />

        {/* Toolbar */}
        <div className="flex flex-wrap items-center justify-between gap-1 p-1 md:flex-nowrap md:gap-2 md:p-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            {/* Attachment Menu */}
            <div className="relative">
              <input
                id={attachFileInputId}
                name="chat-attachments"
                aria-label={t("uploadFilesAria")}
                type="file"
                ref={fileInputRef}
                onChange={handleFileSelect}
                className="hidden"
                multiple
                accept="*/*"
              />
              <input
                id={attachImageInputId}
                name="chat-images"
                aria-label={t("uploadImagesAria")}
                type="file"
                ref={imageInputRef}
                onChange={handleFileSelect}
                className="hidden"
                multiple
                accept="image/*"
              />
              {/* Fallback Input for dumb models */}
              <input
                id={attachTextFallbackInputId}
                name="chat-text-attachments"
                aria-label={t("uploadTextFilesAria")}
                type="file"
                ref={textFallbackInputRef}
                onChange={handleTextFallbackSelect}
                className="hidden"
                multiple
                accept="text/*,application/json,application/xml,application/javascript,application/xhtml+xml,application/x-yaml,application/sql,application/graphql,application/ld+json,application/x-sh,application/x-httpd-php,application/typescript,.csv,.docx,.md,.markdown,.pdf,.pptx,.txt,.xlsx"
              />

              <DropdownMenu
                open={showAttachMenu}
                onOpenChange={(open) => {
                  setShowModelSelect(false);
                  setShowAttachMenu(open);
                }}
              >
                <Tooltip content={t("attach")} position="top">
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("attachFiles")}
                      aria-pressed={hasKnowledgeAttachments}
                      className={`${iconButtonBaseClass} transition-colors ${iconButtonFocusClass} ${
                        showAttachMenu || hasKnowledgeAttachments
                          ? "bg-gray-100 dark:bg-accent text-gray-800 dark:text-foreground"
                          : "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"
                      }`}
                      disabled={isInputBusy}
                    >
                      <Paperclip size={16} aria-hidden="true" />
                    </button>
                  </DropdownMenuTrigger>
                </Tooltip>

                <DropdownMenuContent side="top" align="start" className="w-48">
                  <DropdownMenuItem
                    onSelect={() => {
                      if (modelCapabilities.attachment) {
                        fileInputRef.current?.click();
                      } else {
                        textFallbackInputRef.current?.click();
                      }
                    }}
                  >
                    <FileUp
                      size={14}
                      className="text-blue-500"
                      aria-hidden="true"
                    />
                    <span>{t("uploadFile")}</span>
                  </DropdownMenuItem>
                  {modelCapabilities.vision && (
                    <DropdownMenuItem
                      onSelect={() => {
                        imageInputRef.current?.click();
                      }}
                    >
                      <ImageUp
                        size={14}
                        className="text-green-500"
                        aria-hidden="true"
                      />
                      <span>{t("uploadImage")}</span>
                    </DropdownMenuItem>
                  )}

                  {(modelCapabilities.attachment ||
                    modelCapabilities.vision ||
                    modelCapabilities.audio) && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onSelect={() => {
                          setShowRemoteModal(true);
                        }}
                      >
                        <Link
                          size={14}
                          className="text-purple-500"
                          aria-hidden="true"
                        />
                        <span>{t("remoteFile")}</span>
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {conversationKnowledgeEnabled && (
              <Tooltip content={t("knowledgeBase")} position="top">
                <button
                  type="button"
                  aria-label={t("manageKnowledgeBases")}
                  aria-pressed={normalizedKnowledgeCollectionIds.length > 0}
                  className={`${iconButtonBaseClass} transition-colors ${iconButtonFocusClass} ${
                    normalizedKnowledgeCollectionIds.length > 0
                      ? "bg-purple-50 text-purple-700 dark:bg-purple-950/35 dark:text-purple-200"
                      : "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"
                  }`}
                  disabled={isInputBusy || isSavingKnowledgeSelection}
                  onClick={() => setShowKBModal(true)}
                >
                  <Library size={16} aria-hidden="true" />
                </button>
              </Tooltip>
            )}

            <DropdownMenu
              open={openComposerSection === "tool-mode"}
              onOpenChange={(open) =>
                handleComposerSectionOpenChange("tool-mode", open)
              }
            >
              <Tooltip
                content={
                  toolMode === "agent" && effectiveToolMode === "chat"
                    ? t("agentModeUnsupported")
                    : effectiveToolMode === "agent"
                      ? t("agentModeDescription")
                      : t("chatModeDescription")
                }
                position="top"
              >
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    aria-label={t("toolModeMenuAria", {
                      mode:
                        effectiveToolMode === "agent"
                          ? t("agentMode")
                          : t("chatMode"),
                    })}
                    className={`inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full px-2.5 text-xs font-medium transition-colors ${iconButtonFocusClass} ${
                      effectiveToolMode === "agent"
                        ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/35 dark:text-emerald-200"
                        : "bg-gray-100 text-gray-600 dark:bg-accent/60 dark:text-muted-foreground"
                    }`}
                    disabled={isInputBusy}
                  >
                    {effectiveToolMode === "agent" ? (
                      <Bot size={15} aria-hidden="true" />
                    ) : (
                      <MessageCircle size={15} aria-hidden="true" />
                    )}
                    <span>
                      {effectiveToolMode === "agent"
                        ? t("agentMode")
                        : t("chatMode")}
                    </span>
                    <ChevronDown size={12} aria-hidden="true" />
                  </button>
                </DropdownMenuTrigger>
              </Tooltip>
              <DropdownMenuContent side="top" align="start" className="w-64">
                <DropdownMenuLabel>{t("toolMode")}</DropdownMenuLabel>
                <DropdownMenuRadioGroup
                  value={toolMode}
                  onValueChange={(value) => {
                    if (value === "chat" || value === "agent") {
                      onToolModeChange(value);
                    }
                  }}
                >
                  <DropdownMenuRadioItem
                    value="chat"
                    className={toolModeItemClass}
                  >
                    <MessageCircle
                      size={15}
                      className="mt-0.5 shrink-0"
                      aria-hidden="true"
                    />
                    <span className={toolModeItemTextClass}>
                      <span>{t("chatMode")}</span>
                      <span className={toolModeDescriptionClass}>
                        {t("chatModeDescription")}
                      </span>
                    </span>
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem
                    value="agent"
                    disabled={!canSelectAgentMode}
                    className={toolModeItemClass}
                  >
                    <Bot
                      size={15}
                      className="mt-0.5 shrink-0"
                      aria-hidden="true"
                    />
                    <span className={toolModeItemTextClass}>
                      <span>{t("agentMode")}</span>
                      <span className={toolModeDescriptionClass}>
                        {canSelectAgentMode
                          ? t("agentModeDescription")
                          : t("agentModeUnsupported")}
                      </span>
                    </span>
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>

            {effectiveToolMode === "agent" && showPermissionControl && (
              <DropdownMenu
                open={openComposerSection === "permission"}
                onOpenChange={(open) =>
                  handleComposerSectionOpenChange("permission", open)
                }
              >
                <Tooltip
                  content={t(`permissionDescription.${permissionMode}`)}
                  position="top"
                >
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("permissionMenuAria", {
                        mode: t(`permissionMode.${permissionMode}`),
                      })}
                      className={`inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full px-2.5 text-xs font-medium transition-colors ${iconButtonFocusClass} ${
                        permissionMode === "danger-full-access"
                          ? "bg-red-50 text-red-700 dark:bg-red-950/35 dark:text-red-200"
                          : "bg-amber-50 text-amber-700 dark:bg-amber-950/35 dark:text-amber-200"
                      }`}
                      disabled={
                        isInputBusy ||
                        permissionLocked ||
                        availablePermissionModes.length === 0
                      }
                    >
                      {permissionMode === "danger-full-access" ? (
                        <ShieldAlert size={15} aria-hidden="true" />
                      ) : (
                        <ShieldCheck size={15} aria-hidden="true" />
                      )}
                      <span>{t(`permissionMode.${permissionMode}`)}</span>
                      <ChevronDown size={12} aria-hidden="true" />
                    </button>
                  </DropdownMenuTrigger>
                </Tooltip>
                <DropdownMenuContent side="top" align="start" className="w-72">
                  <DropdownMenuLabel>{t("permissionTitle")}</DropdownMenuLabel>
                  <DropdownMenuRadioGroup
                    value={permissionMode}
                    onValueChange={(value) => {
                      if (
                        value !== "read-only" &&
                        value !== "workspace-write" &&
                        value !== "danger-full-access"
                      ) {
                        return;
                      }
                      if (value === "danger-full-access") {
                        setShowFullAccessConfirmation(true);
                        return;
                      }
                      onPermissionModeChange?.(value, false);
                    }}
                  >
                    {availablePermissionModes.map((mode) => (
                      <DropdownMenuRadioItem
                        key={mode}
                        value={mode}
                        className={toolModeItemClass}
                      >
                        {mode === "danger-full-access" ? (
                          <ShieldAlert
                            size={15}
                            className="mt-0.5 shrink-0 text-red-600"
                            aria-hidden="true"
                          />
                        ) : (
                          <ShieldCheck
                            size={15}
                            className="mt-0.5 shrink-0"
                            aria-hidden="true"
                          />
                        )}
                        <span className={toolModeItemTextClass}>
                          <span>{t(`permissionMode.${mode}`)}</span>
                          <span className={toolModeDescriptionClass}>
                            {t(`permissionDescription.${mode}`)}
                          </span>
                        </span>
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}

            {isReasoningSupported && (
              <DropdownMenu
                open={openComposerSection === "reasoning"}
                onOpenChange={(open) =>
                  handleComposerSectionOpenChange("reasoning", open)
                }
              >
                <Tooltip
                  content={
                    effectiveUseReasoning
                      ? t("reasoningCurrent", {
                          level: reasoningEffortLabel(effectiveReasoningEffort),
                        })
                      : t("reasoningEffortOff")
                  }
                  position="top"
                >
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("reasoningMenuAria", {
                        level: effectiveUseReasoning
                          ? reasoningEffortLabel(effectiveReasoningEffort)
                          : t("reasoningEffortOff"),
                      })}
                      aria-pressed={effectiveUseReasoning}
                      className={`${iconButtonBaseClass} transition-colors ${iconButtonFocusClass} ${effectiveUseReasoning ? "text-violet-500 dark:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-900/20" : "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"}`}
                      disabled={isInputBusy}
                    >
                      <Lightbulb size={16} aria-hidden="true" />
                    </button>
                  </DropdownMenuTrigger>
                </Tooltip>
                <DropdownMenuContent side="top" align="start" className="w-44">
                  <DropdownMenuLabel>{t("reasoningLevel")}</DropdownMenuLabel>
                  <DropdownMenuRadioGroup
                    value={
                      effectiveUseReasoning ? effectiveReasoningEffort : "off"
                    }
                    onValueChange={updateReasoningSelection}
                  >
                    <DropdownMenuRadioItem
                      value="off"
                      indicatorPosition="right"
                      indicator={
                        <Check size={14} strokeWidth={2.5} aria-hidden="true" />
                      }
                      className={reasoningEffortItemClass}
                    >
                      {t("reasoningEffortOff")}
                    </DropdownMenuRadioItem>
                    {reasoningEffortOptions.map((effort) => (
                      <DropdownMenuRadioItem
                        key={effort}
                        value={effort}
                        indicatorPosition="right"
                        indicator={
                          <Check
                            size={14}
                            strokeWidth={2.5}
                            aria-hidden="true"
                          />
                        }
                        className={reasoningEffortItemClass}
                      >
                        {reasoningEffortLabel(effort)}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}

            {/* Search Button */}
            {onSearchModeChange && (
              <DropdownMenu
                open={openComposerSection === "search"}
                onOpenChange={(open) =>
                  handleComposerSectionOpenChange("search", open)
                }
              >
                <Tooltip content={searchTooltip} position="top">
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("searchModeMenuAria", {
                        mode:
                          searchMode === "external"
                            ? searchModeLabel
                            : searchMode === "model_builtin"
                              ? modelBuiltInSearchLabel
                              : t("searchModeOff"),
                      })}
                      aria-pressed={isSearchEnabled}
                      className={`${iconButtonBaseClass} transition-colors ${iconButtonFocusClass} ${
                        isSearchEnabled
                          ? "text-blue-500 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20"
                          : "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"
                      }`}
                      disabled={isInputBusy}
                    >
                      <Globe size={16} aria-hidden="true" />
                    </button>
                  </DropdownMenuTrigger>
                </Tooltip>
                <DropdownMenuContent side="top" align="start" className="w-64">
                  <DropdownMenuLabel>{t("searchMode")}</DropdownMenuLabel>
                  <DropdownMenuRadioGroup
                    value={searchMode}
                    onValueChange={handleSearchModeChange}
                  >
                    <DropdownMenuRadioItem
                      value="off"
                      indicatorPosition="right"
                      indicator={
                        <Check size={14} strokeWidth={2.5} aria-hidden="true" />
                      }
                      className={searchModeItemClass}
                    >
                      {t("searchModeOff")}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem
                      value="model_builtin"
                      disabled={!modelBuiltInSearch.enabled}
                      indicatorPosition="right"
                      indicator={
                        <Check size={14} strokeWidth={2.5} aria-hidden="true" />
                      }
                      className={searchModeItemClass}
                    >
                      <span className="flex min-w-0 flex-col">
                        <span>{modelBuiltInSearchLabel}</span>
                        {!modelBuiltInSearch.enabled && (
                          <span className="truncate text-[10px] font-normal text-muted-foreground">
                            {modelBuiltInUnavailableMessage}
                          </span>
                        )}
                      </span>
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem
                      value="external"
                      disabled={!searchCompatibility.enabled}
                      indicatorPosition="right"
                      indicator={
                        <Check size={14} strokeWidth={2.5} aria-hidden="true" />
                      }
                      className={searchModeItemClass}
                    >
                      {searchModeLabel}
                    </DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>

          <div className="flex shrink-0 items-center gap-0.5">
            {/* Model Selector */}
            <div className="relative">
              <DropdownMenu
                open={showModelSelect && availableModels.length > 0}
                onOpenChange={(open) => {
                  setShowAttachMenu(false);
                  setShowModelSelect(open && availableModels.length > 0);
                }}
              >
                <Tooltip content={currentModelName} position="top">
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("selectModelAria", {
                        model: currentModelName,
                      })}
                      className={`group inline-flex h-8 w-8 shrink-0 items-center justify-center gap-1.5 rounded-lg px-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 md:w-auto md:max-w-52 dark:text-muted-foreground dark:hover:bg-accent/50 dark:hover:text-foreground ${iconButtonFocusClass}`}
                      disabled={isInputBusy}
                    >
                      {/* Mobile: Just Icon */}
                      <Cpu size={16} className="md:hidden" aria-hidden="true" />

                      {/* Desktop: Text + Chevron */}
                      <div className="hidden min-w-0 items-center gap-0.5 md:flex">
                        <span className="max-w-44 truncate text-xs font-medium">
                          {truncateMiddle(currentModelName, 30)}
                        </span>
                        <ChevronDown
                          size={12}
                          aria-hidden="true"
                          className={`opacity-50 transition-[opacity,transform] duration-200 group-hover:opacity-100 ${showModelSelect ? "rotate-180" : ""}`}
                        />
                      </div>
                    </button>
                  </DropdownMenuTrigger>
                </Tooltip>

                <DropdownMenuContent
                  side="top"
                  align="end"
                  className="max-h-64 w-56 overflow-y-auto custom-scrollbar"
                >
                  <DropdownMenuRadioGroup
                    value={selectedModel}
                    onValueChange={(model) => {
                      onSelectModel?.(model);
                      setShowModelSelect(false);
                    }}
                  >
                    {(
                      Object.entries(groupedModels) as [string, ModelInfo[]][]
                    ).map(([providerName, models]) => (
                      <div key={providerName}>
                        <DropdownMenuLabel>{providerName}</DropdownMenuLabel>
                        {models.map((model) => (
                          <DropdownMenuRadioItem
                            value={model.name}
                            aria-label={t("useModelAria", {
                              model: model.displayName,
                            })}
                            indicatorPosition="right"
                            key={model.name}
                            className={
                              selectedModel === model.name
                                ? "font-medium text-brand"
                                : undefined
                            }
                          >
                            <span className="truncate">
                              {model.displayName}
                            </span>
                          </DropdownMenuRadioItem>
                        ))}
                      </div>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {/* Text Polish Button */}
            <div>
              <Tooltip
                content={
                  isPolishingInput ? t("polishingText") : t("polishText")
                }
                position="top"
              >
                <button
                  type="button"
                  aria-label={t("polishTextAria")}
                  aria-busy={isPolishingInput || undefined}
                  disabled={isInputBusy || isPolishingInput || !input.trim()}
                  className={`${iconButtonBaseClass} transition-colors ${iconButtonFocusClass} ${
                    input.trim()
                      ? "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"
                      : "text-gray-300 dark:text-muted-foreground/40"
                  } disabled:cursor-not-allowed disabled:opacity-50`}
                  onClick={handlePolishInput}
                >
                  {isPolishingInput ? (
                    <Loader2
                      size={16}
                      className="animate-spin"
                      aria-hidden="true"
                    />
                  ) : (
                    <PencilSparkles size={16} aria-hidden="true" />
                  )}
                </button>
              </Tooltip>
            </div>

            {/* Actions */}
            <div className="flex shrink-0 items-center gap-1">
              {isInputBusy ? (
                onStop && !isParsingAttachments ? (
                  <Tooltip content={t("stopGeneration")} position="top">
                    <button
                      type="button"
                      aria-label={t("stopGenerationAria")}
                      aria-busy="true"
                      className={`${iconButtonBaseClass} relative overflow-hidden bg-gray-100 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-500 dark:bg-accent dark:text-muted-foreground dark:hover:bg-red-900/20 dark:hover:text-red-400 group ${iconButtonFocusClass}`}
                      onClick={onStop}
                    >
                      <div className="relative w-4 h-4">
                        <Loader2
                          size={16}
                          aria-hidden="true"
                          className="animate-spin absolute inset-0 transition-[opacity,transform] duration-300 group-hover:opacity-0 group-hover:scale-75"
                        />
                        <Square
                          size={16}
                          fill="currentColor"
                          aria-hidden="true"
                          className="absolute inset-0 opacity-0 scale-75 transition-[opacity,transform] duration-300 group-hover:opacity-100 group-hover:scale-100"
                        />
                      </div>
                    </button>
                  </Tooltip>
                ) : (
                  <button
                    type="button"
                    aria-label={t("working")}
                    aria-busy="true"
                    className={`${iconButtonBaseClass} cursor-not-allowed bg-transparent text-gray-500 dark:text-muted-foreground`}
                  >
                    <Loader2
                      size={16}
                      className="animate-spin"
                      aria-hidden="true"
                    />
                  </button>
                )
              ) : input || attachments.length > 0 ? (
                <Tooltip content={t("sendMessage")} position="top">
                  <button
                    type="button"
                    aria-label={t("sendMessageAria")}
                    disabled={!selectedModel || isParsingAttachments}
                    className={`${iconButtonBaseClass} bg-gray-100 text-gray-500 shadow-sm transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-accent dark:text-muted-foreground dark:hover:bg-accent/80 ${iconButtonFocusClass}`}
                    onClick={handleSend}
                  >
                    <SendHorizontal size={16} aria-hidden="true" />
                  </button>
                </Tooltip>
              ) : (
                <div className="relative">
                  {isRecording && (
                    <div
                      className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-red-600 text-white text-xs font-bold rounded-full animate-pulse whitespace-nowrap shadow-md dark:bg-red-500"
                      aria-hidden="true"
                    >
                      {formatTime(recordingSeconds)}
                    </div>
                  )}

                  <Tooltip
                    content={
                      isRecording
                        ? t("stopRecording")
                        : voice.autoTranscribe
                          ? t("speechToText")
                          : t("voiceMessage")
                    }
                    position="top"
                  >
                    <button
                      type="button"
                      aria-label={
                        isRecording
                          ? t("stopRecordingAria", {
                              time: formatTime(recordingSeconds),
                            })
                          : voice.autoTranscribe
                            ? t("speechToTextAria")
                            : t("voiceMessageAria")
                      }
                      aria-pressed={isRecording}
                      className={`${iconButtonBaseClass} transition-[background-color,color,box-shadow] ${iconButtonFocusClass} ${isRecording ? "bg-red-50 dark:bg-red-900/30 text-red-500 dark:text-red-400 ring-1 ring-red-200 dark:ring-red-800" : "text-gray-500 dark:text-muted-foreground hover:text-gray-700 dark:hover:text-foreground hover:bg-gray-100 dark:hover:bg-accent/50"}`}
                      onClick={toggleRecording}
                      onContextMenu={(e) => {
                        e.preventDefault();
                        updateVoiceSettings({
                          autoTranscribe: !voice.autoTranscribe,
                        });
                      }}
                    >
                      {isRecording ? (
                        <StopCircle size={16} aria-hidden="true" />
                      ) : (
                        <Mic size={16} aria-hidden="true" />
                      )}
                    </button>
                  </Tooltip>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    );
  },
);

MessageInput.displayName = "MessageInput";

export default MessageInput;
