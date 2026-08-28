import { expect, test } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";

const conversationA = "20000000-0000-4000-8000-000000000201";
const conversationB = "20000000-0000-4000-8000-000000000202";

test("each conversation owns and restores its selected model", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(conversationA, "Model chat A", "gpt-5.6-luna"),
    conversation(conversationB, "Model chat B", "gpt-5.6-terra"),
  ]);
  await api.authenticate(page);
  await api.install(page);

  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "Model chat A", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel(/选择模型：.*Luna/i)).toBeVisible();

  await page.getByRole("button", { name: "Model chat B", exact: true }).click();
  await expect(page.getByLabel(/选择模型：.*Terra/i)).toBeVisible();

  await page.getByLabel(/选择模型：.*Terra/i).click();
  await page.getByLabel(/使用 .*5\.5/i).click();
  await expect
    .poll(() => api.conversations[1].modelRef.modelId)
    .toBe("gpt-5.5");
  await expect(page.getByLabel(/选择模型：.*5\.5/i)).toBeVisible();

  await page.getByRole("button", { name: "Model chat A", exact: true }).click();
  await expect(page.getByLabel(/选择模型：.*Luna/i)).toBeVisible();
  expect(api.conversations[0].modelRef.modelId).toBe("gpt-5.6-luna");

  await page.reload();
  await expect(
    page.getByRole("button", { name: "Model chat B", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Model chat B", exact: true }).click();
  await expect(page.getByLabel(/选择模型：.*5\.5/i)).toBeVisible();
  await page.getByRole("button", { name: "Model chat A", exact: true }).click();
  await expect(page.getByLabel(/选择模型：.*Luna/i)).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});
