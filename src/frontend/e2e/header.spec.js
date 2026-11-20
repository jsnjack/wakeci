import { test, expect } from "./fixtures.js";

test("should open the pages from the menu on a phone", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");
    await expect(page.getByTestId("page-title")).toHaveText("Feed");
    // The links don't fit; the menu has them
    await expect(page.getByTestId("nav-jobs")).toBeHidden();
    await page.getByTestId("nav-menu").click();
    await page.getByRole("menuitem", { name: "Jobs" }).click();
    await expect(page).toHaveURL(/\/jobs$/);
    await expect(page.getByRole("menu")).toHaveCount(0);
    // Esc and a click outside close it too
    await page.getByTestId("nav-menu").click();
    await expect(page.getByRole("menu")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu")).toHaveCount(0);
    await page.getByTestId("nav-menu").click();
    await page.getByTestId("page-title").click();
    await expect(page.getByRole("menu")).toHaveCount(0);
});

test("should mark the open page in the header", async ({ page }) => {
    await page.goto("/jobs");
    await expect(page.getByTestId("nav-jobs")).toHaveAttribute("aria-current", "page");
    await expect(page.getByTestId("nav-feed")).not.toHaveAttribute("aria-current", "page");
});

test("should follow the system's theme until the user picks one", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "dark" });
    await page.goto("/");
    await expect(page.locator("body")).toHaveClass(/\bdark\b/);
    await page.emulateMedia({ colorScheme: "light" });
    await expect(page.locator("body")).toHaveClass(/\blight\b/);

    // Picked, it stays, whatever the system says
    await page.getByTestId("toggle-theme").click();
    await expect(page.locator("body")).toHaveClass(/\bdark\b/);
    await page.emulateMedia({ colorScheme: "light" });
    await page.reload();
    await expect(page.locator("body")).toHaveClass(/\bdark\b/);
});
