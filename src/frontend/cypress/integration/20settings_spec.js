describe("Settings page", function() {
    it("should modify settings", function() {
        cy.visit("/settings");
        cy.login();
        cy.get("input[type=password]").clear();
        cy.get("input[type=password]").type("secret");

        cy.get("#concurrent-builds").clear().type("100");

        cy.get("#build-history-size").clear().type("100");

        cy.get("[data-cy=save-settings]").click();
        cy.get(".notification-content").should("contain", "Saved");
        cy.get("[data-cy=logout]").click();

        cy.login("secret");

        cy.visit("/settings");
        cy.get(".loading").should("not.exist");
        cy.get("input[type=password]").clear();
        cy.get("input[type=password]").type("admin");

        cy.get("#concurrent-builds").clear().type("2");

        cy.get("#build-history-size").clear().type("200");

        cy.get("[data-cy=save-settings]").click();
    });
})
;
