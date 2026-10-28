describe("Login page", function () {
    it("should not offer single sign-on when it isn't configured", function () {
        cy.intercept("GET", "/auth/methods").as("methods");
        cy.visit("/login");
        cy.wait("@methods").its("response.body.sso").should("eq", false);
        cy.get("input#password").should("be.visible");
        cy.get("[data-cy=sso-login]").should("not.exist");
    });

    it("should offer single sign-on and keep the redirect", function () {
        cy.intercept("GET", "/auth/methods", { body: { sso: true } });
        cy.visit("/login?redirect=/jobs");
        cy.get("[data-cy=sso-login]")
            .should("have.attr", "href", "/auth/sso/login?redirect=%2Fjobs");
        cy.get("input#password").should("be.visible");
    });

    it("should explain a rejected account and keep the redirect", function () {
        cy.intercept("GET", "/auth/methods", { body: { sso: true } });
        cy.visit("/login?error=sso_not_allowed&redirect=%2Fjobs");
        cy.get(".notification-content").should("contain", "not in a group that is allowed to log in");
        cy.location("search").should("not.contain", "error");
        cy.get("[data-cy=sso-login]")
            .should("have.attr", "href", "/auth/sso/login?redirect=%2Fjobs");
    });

    it("should ignore unknown error codes", function () {
        cy.visit("/login?error=constructor");
        cy.get("input#password").should("be.visible");
        cy.location("search").should("eq", "");
        cy.get(".notification-content").should("not.exist");
    });
});
