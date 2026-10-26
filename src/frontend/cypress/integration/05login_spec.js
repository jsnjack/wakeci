describe("Login page", function () {
    it("should not offer Google sign-in when it isn't configured", function () {
        cy.intercept("GET", "/auth/methods").as("methods");
        cy.visit("/login");
        cy.wait("@methods").its("response.body.google").should("eq", false);
        cy.get("input#password").should("be.visible");
        cy.get("[data-cy=google-login]").should("not.exist");
    });

    it("should offer Google sign-in and keep the redirect", function () {
        cy.intercept("GET", "/auth/methods", { body: { google: true } });
        cy.visit("/login?redirect=/jobs");
        cy.get("[data-cy=google-login]")
            .should("have.attr", "href", "/auth/google/login?redirect=%2Fjobs");
        cy.get("input#password").should("be.visible");
    });

    it("should explain a rejected Google account and keep the redirect", function () {
        cy.intercept("GET", "/auth/methods", { body: { google: true } });
        cy.visit("/login?error=google_domain&redirect=%2Fjobs");
        cy.get(".notification-content").should("contain", "not allowed to log in");
        cy.location("search").should("not.contain", "error");
        cy.get("[data-cy=google-login]")
            .should("have.attr", "href", "/auth/google/login?redirect=%2Fjobs");
    });

    it("should ignore unknown error codes", function () {
        cy.visit("/login?error=constructor");
        cy.get("input#password").should("be.visible");
        cy.location("search").should("eq", "");
        cy.get(".notification-content").should("not.exist");
    });
});
