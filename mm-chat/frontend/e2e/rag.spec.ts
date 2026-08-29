import { expect, test } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";

const conversationId = "20000000-0000-4000-8000-000000000401";
const collectionId = "70000000-0000-4000-8000-000000000001";
const collectionName = "RAG E2E 验收库";
const fileName = "rag-e2e-acceptance.txt";

test("knowledge upload moves from processing to available", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "RAG upload chat", "gpt-5.6-luna"),
  ]);
  api.knowledge.seedCollection({ id: collectionId, name: collectionName });
  api.knowledge.queueUpload({
    fileName,
    mimeType: "text/plain",
    size: 57,
  });
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByRole("button", { name: "打开知识库" }).click();
  await page
    .getByRole("button", { name: `打开合集 ${collectionName}` })
    .click();

  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: /选择文件或拖拽到此处/ }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles({
    name: fileName,
    mimeType: "text/plain",
    buffer: Buffer.from("Neo Chat RAG E2E 验收暗号：苍蓝回声。", "utf8"),
  });

  await expect(
    page.getByText("已提交 1 个文档进入索引队列。", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText(fileName, { exact: true })).toBeVisible();
  await expect(page.getByText("处理中", { exact: true })).toBeVisible();

  const document = api.knowledge.documents.get(collectionId)?.[0];
  expect(document).toBeDefined();
  api.knowledge.activateDocument(document!.id);
  await page.getByRole("button", { name: "刷新文档" }).click();

  await expect(page.getByText("可用", { exact: true })).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});

test("a selected collection produces a grounded answer with citation", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "RAG cited chat", "gpt-5.6-luna"),
  ]);
  api.knowledge.seedCollection({ id: collectionId, name: collectionName });
  api.knowledge.seedActiveDocument(collectionId, fileName);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByRole("button", { name: "管理当前聊天的知识库" }).click();
  await page
    .getByRole("button", { name: `选择合集 ${collectionName}` })
    .click();
  await page.getByRole("button", { name: "保存选择" }).click();
  await expect
    .poll(
      () =>
        api.conversations[0].config?.selectedKnowledgeCollectionIds as
          string[] | undefined,
    )
    .toEqual([collectionId]);

  await page.getByLabel("消息").fill("验收暗号是什么？");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationId);
  api.completeKnowledgeRun(conversationId, "验收暗号是“苍蓝回声”。[K1]", {
    outcome: "answered",
    selectedCollectionIds: [collectionId],
    citationCount: 1,
    evidenceUsed: true,
    citations: [
      {
        id: "citation-rag-e2e",
        marker: "[K1]",
        snippet: "Neo Chat RAG E2E 验收暗号：苍蓝回声。",
        collectionId,
        sourceName: fileName,
        displayLocator: {
          kind: "line_range",
          startLine: 1,
          endLine: 1,
        },
      },
    ],
  });

  await expect(page.getByText(/验收暗号是“苍蓝回声”/)).toBeVisible();
  await page
    .getByRole("button", { name: "知识引用（1 条）", exact: true })
    .click();
  await expect(page.getByText(new RegExp(fileName))).toBeVisible();
  await expect(
    page.getByText("Neo Chat RAG E2E 验收暗号：苍蓝回声。", {
      exact: true,
    }),
  ).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});

test("knowledge dependency failure renders a terminal degraded answer", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "RAG degraded chat", "gpt-5.6-luna", {
      config: { selectedKnowledgeCollectionIds: [collectionId] },
    }),
  ]);
  api.knowledge.seedCollection({ id: collectionId, name: collectionName });
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("从知识库回答这个问题");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationId);
  api.completeKnowledgeRun(conversationId, "当前只能给出普通回答。", {
    outcome: "dependency_unavailable",
    selectedCollectionIds: [collectionId],
    citationCount: 0,
    evidenceUsed: false,
    degradationReason: "fixture_dependency_unavailable",
    citations: [],
  });

  await expect(
    page.getByText("知识库暂时不可用，本回答未使用知识库。", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByText("当前只能给出普通回答。", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("RAG degraded chat 正在运行")).toHaveCount(0);
  await expect(page.getByText(/知识引用（/)).toHaveCount(0);
  expect(api.unhandledRequests).toEqual([]);
});
