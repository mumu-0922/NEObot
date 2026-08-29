import { expect, test } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";

const conversationId = "20000000-0000-4000-8000-000000000501";
const memoryContent = "用户喜欢喝生椰拿铁。";

test("server Memory governance persists policy and a Global fact", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "Memory governance chat", "gpt-5.6-luna"),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/?panel=settings&settingsTab=memory");

  await expect(
    page.getByRole("status", { name: "记忆服务状态" }),
  ).toContainText("可用");
  const globalUse = page.getByRole("checkbox", { name: "全局 Use" });
  await globalUse.focus();
  await globalUse.press("Space");
  await expect
    .poll(() => api.memory.snapshot.settings.searchEnabled)
    .toBe(false);

  await page.getByLabel("记忆内容").fill(memoryContent);
  await page.getByRole("button", { name: "添加", exact: true }).click();
  await expect(page.getByText(memoryContent, { exact: true })).toBeVisible();
  expect(api.memory.snapshot.memories).toHaveLength(1);

  await page.reload();
  await expect(page.getByText(memoryContent, { exact: true })).toBeVisible();
  await expect(
    page.getByRole("checkbox", { name: "全局 Use" }),
  ).not.toBeChecked();
  expect(api.memory.snapshot.settings.searchEnabled).toBe(false);
  expect(api.unhandledRequests).toEqual([]);
});

test("an explicit recall renders the Memory search step and grounded answer", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "Memory recall chat", "gpt-5.6-luna"),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("你还记得我喜欢喝什么吗？");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationId);
  api.completeMemoryRecallRun(
    conversationId,
    "你喜欢喝生椰拿铁。",
    memoryContent,
  );

  await expect(page.getByText("Memory search", { exact: true })).toBeVisible();
  await expect(
    page.getByText("你喜欢喝生椰拿铁。", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Memory recall chat 正在运行")).toHaveCount(0);
  expect(api.unhandledRequests).toEqual([]);
});

test("a direct Memory action exposes revision-fenced undo", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationId, "Memory action chat", "gpt-5.6-luna"),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("记住我喜欢喝生椰拿铁");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationId);
  const activityId = api.completeMemoryActionRun(
    conversationId,
    "已经记住了。",
    memoryContent,
  );

  const summary = page.getByRole("button", { name: /记忆活动：新增 1/ });
  await expect(summary).toBeVisible();
  await summary.click();
  await expect(page.getByText(memoryContent, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "撤销本次记忆变更" }).click();

  await expect(
    page.getByRole("button", { name: "撤销本次记忆变更" }),
  ).toHaveCount(0);
  expect(
    [...api.memory.activities.values()]
      .flat()
      .find((activity) => activity.id === activityId)?.undoStatus,
  ).toBe("undone");
  expect(api.memory.snapshot.memories).toHaveLength(0);
  expect(api.unhandledRequests).toEqual([]);
});
