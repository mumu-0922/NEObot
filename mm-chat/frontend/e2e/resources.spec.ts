import { expect, test, type Locator } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";
import { TEST_MCP_SERVER, TEST_SKILL } from "./fixtures/neoChatResourceApi";

const conversationA = "20000000-0000-4000-8000-000000000601";
const conversationB = "20000000-0000-4000-8000-000000000602";
const skillURL = "https://lobehub.com/skills/e2e-report-skill";

test("Skill installation updates Library without selecting a Conversation", async ({
  page,
}) => {
  const api = resourceApi();
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByRole("button", { name: "打开 Skill" }).click();
  await page.getByRole("tab", { name: "Skill", exact: true }).click();
  await page.getByLabel("Skill 精确链接").fill(skillURL);
  await page.getByRole("button", { name: "安装", exact: true }).click();

  await expect(
    page.getByText("已安装 e2e-report-skill。", { exact: true }),
  ).toBeAttached();
  await expect(
    page.getByRole("heading", { name: TEST_SKILL.name, exact: true }),
  ).toBeVisible();
  expect(api.resources.lastInstalledSkillURL).toBe(skillURL);
  expect(
    api.resources.getSkillSelection(conversationA).installationIds,
  ).toEqual([]);
  expect(
    api.resources.getSkillSelection(conversationB).installationIds,
  ).toEqual([]);
  expect(api.unhandledRequests).toEqual([]);
});

test("Skill and MCP selections persist per Conversation across reload", async ({
  page,
}) => {
  const api = resourceApi();
  api.resources.seedSkill();
  api.resources.seedMcpServer();
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("选择当前对话可用的 Skill").click();
  await page
    .getByRole("menuitemcheckbox", { name: new RegExp(TEST_SKILL.name) })
    .click();
  await expect
    .poll(() => api.resources.getSkillSelection(conversationA))
    .toMatchObject({ revision: 1, installationIds: [TEST_SKILL.id] });

  await page.getByLabel("选择当前对话可用的 MCP").click();
  await page
    .getByRole("menuitemcheckbox", { name: new RegExp(TEST_MCP_SERVER.name) })
    .click();
  await expect
    .poll(() => api.resources.getMcpSelection(conversationA))
    .toMatchObject({
      mode: "custom",
      revision: 1,
      servers: [{ ref: TEST_MCP_SERVER.ref, disabledTools: [] }],
    });

  await page.reload();
  await expect(page.getByLabel("选择当前对话可用的 Skill")).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.getByLabel("选择当前对话可用的 MCP")).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await page
    .getByRole("button", { name: "Resource chat B", exact: true })
    .click();
  await expect(page.getByLabel("选择当前对话可用的 Skill")).toHaveAttribute(
    "aria-pressed",
    "false",
  );
  await expect(page.getByLabel("选择当前对话可用的 MCP")).toHaveAttribute(
    "aria-pressed",
    "false",
  );
  expect(
    api.resources.getSkillSelection(conversationB).installationIds,
  ).toEqual([]);
  expect(api.resources.getMcpSelection(conversationB).servers).toEqual([]);
  expect(api.unhandledRequests).toEqual([]);
});

test("Agent transcript keeps Skill and MCP work before the final answer", async ({
  page,
}) => {
  const api = resourceApi();
  api.resources.seedSkill();
  api.resources.seedMcpServer();
  selectResources(api, conversationA);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("用 Skill 和 MCP 检查问题");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationA);
  const assistantId = api.completeResourceRun(
    conversationA,
    "Skill 与 MCP 检查已经完成。",
  );

  const assistant = page.locator(`#chat-message-${assistantId}`);
  await expect(assistant).toContainText("先运行已选择的 Skill。");
  await expect(assistant).toContainText(TEST_SKILL.name);
  await expect(assistant).toContainText("再调用已选择的 MCP。");
  await expect(assistant).toContainText("E2E Issues · Lookup issue");
  await expect(assistant).toContainText("Skill 与 MCP 检查已经完成。");
  await assertResourceOrder(assistant);

  await page.reload();
  const reloaded = page.locator(`#chat-message-${assistantId}`);
  await expect(reloaded).toContainText("E2E Issues · Lookup issue");
  await assertResourceOrder(reloaded);
  expect(api.unhandledRequests).toEqual([]);
});

test("failed MCP execution reaches a stable terminal answer", async ({
  page,
}) => {
  const api = resourceApi("Resource failure chat");
  api.resources.seedSkill();
  api.resources.seedMcpServer();
  selectResources(api, conversationA);
  await api.authenticate(page);
  await api.install(page);
  await page.goto("/");

  await page.getByLabel("消息").fill("触发 MCP 失败");
  await page.getByLabel("发送消息").click();
  await api.waitForPendingRun(conversationA);
  const assistantId = api.completeResourceRun(
    conversationA,
    "MCP 暂时不可用，任务已安全结束。",
    "failed",
  );

  const assistant = page.locator(`#chat-message-${assistantId}`);
  await expect(assistant).toContainText("E2E Issues · Lookup issue");
  await expect(assistant).toContainText("错误");
  await expect(assistant).toContainText("MCP 暂时不可用，任务已安全结束。");
  await expect(page.getByLabel("Resource failure chat 正在运行")).toHaveCount(
    0,
  );
  expect(api.unhandledRequests).toEqual([]);
});

function resourceApi(titleA = "Resource chat A") {
  return new NeoChatApiFixture([
    conversation(conversationA, titleA, "gpt-5.6-luna"),
    conversation(conversationB, "Resource chat B", "gpt-5.6-terra"),
  ]);
}

function selectResources(api: NeoChatApiFixture, conversationId: string) {
  api.resources.skillSelections.set(conversationId, {
    revision: 1,
    installationIds: [TEST_SKILL.id],
  });
  api.resources.mcpSelections.set(conversationId, {
    mode: "custom",
    revision: 1,
    servers: [{ ref: TEST_MCP_SERVER.ref, disabledTools: [] }],
  });
}

async function assertResourceOrder(locator: Locator) {
  const process = locator.getByRole("region", { name: "Agent 执行过程" });
  const blocks = process.getByRole("listitem");
  await expect(blocks).toHaveCount(4);
  await expect(blocks.nth(0)).toHaveText("先运行已选择的 Skill。");
  await expect(blocks.nth(1)).toContainText(TEST_SKILL.name);
  await expect(blocks.nth(2)).toHaveText("再调用已选择的 MCP。");
  await expect(blocks.nth(3)).toContainText("E2E Issues · Lookup issue");
  await expect(
    locator.getByText("Skill 与 MCP 检查已经完成。", { exact: true }),
  ).toBeVisible();
}
