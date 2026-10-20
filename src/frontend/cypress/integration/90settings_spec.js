describe("Settings page", function () {
    it("should modify settings", function () {
        cy.visit("/settings");
        cy.login();
        cy.location("pathname").should("eq", "/settings");
        cy.get("input[type=password]").clear();
        cy.get("input[type=password]").type("secret");

        cy.get("#concurrent-builds").clear().type("100");

        cy.get("#build-history-size").clear().type("100");

        cy.get("[data-cy=save-settings]").click();
        cy.get(".notification-content").should("contain", "Saved");
        cy.get("[data-cy=logout]").click();
        cy.location("pathname").should("eq", "/login");

        cy.login("secret");

        cy.visit("/settings");
        cy.location("pathname").should("eq", "/settings");
        cy.get(".loading").should("not.exist");
        cy.get("input[type=password]").clear();
        cy.get("input[type=password]").type("admin");

        cy.get("#concurrent-builds").clear().type("2");

        cy.get("#build-history-size").clear().type("200");

        cy.get("[data-cy=save-settings]").click();
        cy.get(".notification-content").should("contain", "Saved");
    });
});


describe("Settings with a session that survives password changes", function () {
    it("does not submit the previous password again on later saves", function () {
        cy.visit("/settings");
        cy.login();
        cy.get("input[type=password]").should("be.visible");
        cy.intercept("POST", "/api/settings", { statusCode: 200, body: "" }).as("saveSettings");
        cy.get("input[type=password]").type("do-not-save-again");
        cy.get("[data-cy=save-settings]").click();
        cy.wait("@saveSettings");
        cy.get("input[type=password]").should("have.value", "");
        cy.get("#concurrent-builds").clear().type("3");
        cy.get("[data-cy=save-settings]").click();
        cy.wait("@saveSettings").then(({ request }) => {
            const body = typeof request.body === "string" ? request.body : JSON.stringify(request.body);
            expect(body).not.to.contain("do-not-save-again");
        });
    });
});
