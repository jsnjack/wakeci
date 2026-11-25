import { test, expect } from "./fixtures.js";

test("an address that matches no page says so and leads back", async ({ page }) => {
    await page.goto("/nope/nothing");
    await expect(page.getByTestId("not-found").getByRole("heading")).toHaveText("There's no such page");
    await expect(page.getByTestId("page-title")).toHaveText("Not found");
    await page.getByRole("link", { name: "Open the feed" }).click();
    await expect(page).toHaveURL("/");
});

test("a build that doesn't exist says which", async ({ page }) => {
    await page.goto("/build/999999");
    await expect(page.getByTestId("not-found").getByRole("heading")).toHaveText("Build #999999 isn't here");
});
