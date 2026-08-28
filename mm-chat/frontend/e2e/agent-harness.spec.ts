import { expect, test } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";

const agentA = "20000000-0000-4000-8000-000000000301";
const agentB = "20000000-0000-4000-8000-000000000302";

test("two conversations in one workspace run independently", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(agentA, "Agent chat A", "gpt-5.6-luna"),
    conversation(agentB, "Agent chat B", "gpt-5.6-terra"),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("执行任务 A");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(agentA);
  await expect(page.getByLabel("Agent chat A 正在运行")).toBeVisible();

  await page.getByRole("button", { name: "Agent chat B", exact: true }).click();
  await page.getByLabel("消息").fill("执行任务 B");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(agentB);

  await expect(page.getByLabel("Agent chat A 正在运行")).toBeVisible();
  await expect(page.getByLabel("Agent chat B 正在运行")).toBeVisible();

  api.completeRun(agentA, "任务 A 已完成");
  api.completeRun(agentB, "任务 B 已完成");
  await expect(page.getByText("任务 B 已完成", { exact: true })).toBeVisible();

  await page.getByText("Agent chat A", { exact: true }).click();
  await expect(page.getByText("任务 A 已完成", { exact: true })).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});

test("a failed run becomes a recoverable terminal message", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(agentA, "Failing agent", "gpt-5.6-luna"),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("触发失败");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(agentA);
  api.failRun(agentA);

  await expect(page.getByText("生成失败", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Failing agent 正在运行")).toHaveCount(0);
  expect(api.unhandledRequests).toEqual([]);
});

test("an active server run can be cancelled from its owning conversation", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(agentA, "Cancelable agent", "gpt-5.6-luna", {
      activeGeneration: {
        runId: "run-cancel-e2e",
        messageId: "32000000-0000-4000-8000-000000000001",
        status: "streaming",
      },
    }),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await expect(page.getByLabel("Cancelable agent 正在运行")).toBeVisible();
  await page.getByLabel("停止生成").click();

  await expect(page.getByText("任务已停止。", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Cancelable agent 正在运行")).toHaveCount(0);
  expect(api.unhandledRequests).toEqual([]);
});

test("refresh restores the durable terminal state after a detached run", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(agentA, "Recovering agent", "gpt-5.6-luna", {
      activeGeneration: {
        runId: "run-refresh-e2e",
        messageId: "33000000-0000-4000-8000-000000000001",
        status: "streaming",
      },
    }),
  ]);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");
  await expect(page.getByLabel("Recovering agent 正在运行")).toBeVisible();

  delete api.conversations[0].activeGeneration;
  api.seedTerminalAnswer(agentA, "刷新后恢复的最终结果");
  await page.reload();

  await expect(
    page.getByText("刷新后恢复的最终结果", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Recovering agent 正在运行")).toHaveCount(0);
  expect(api.unhandledRequests).toEqual([]);
});

test("durable Agent steps stay interleaved and workspace output opens in app", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(agentA, "Artifact agent", "gpt-5.6-luna"),
  ]);
  const assistantId = api.seedAgentArtifact(agentA);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  const message = page.locator(`#chat-message-${assistantId}`);
  await expect(message).toContainText("先读取项目状态。");
  await expect(message).toContainText("pwd");
  await expect(message).toContainText("文件已经写入工作区。");
  await expect(message).toContainText("报告已经生成。");

  const text = (await message.textContent()) ?? "";
  expect(text.indexOf("先读取项目状态。")).toBeLessThan(text.indexOf("pwd"));
  expect(text.indexOf("pwd")).toBeLessThan(
    text.indexOf("文件已经写入工作区。"),
  );
  expect(text.indexOf("文件已经写入工作区。")).toBeLessThan(
    text.indexOf("报告已经生成。"),
  );

  await page.getByLabel("打开工作区文件 e2e-report.txt").click();
  await expect(
    page.getByRole("dialog", { name: "e2e-report.txt" }),
  ).toBeVisible();
  await expect(
    page.getByText("deterministic report", { exact: true }),
  ).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});
