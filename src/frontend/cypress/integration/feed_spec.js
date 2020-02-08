describe("Feed page", function() {
    it("should open Feed page", function() {
        cy.visit("http://localhost:8080/");
        cy.login();
        cy.get("a[href='/']").should("contain", "Feed");
        cy.get("a[href='/jobs']").should("contain", "Jobs");
        cy.get("a[href='/settings']").should("contain", "Settings");
    });
})
;
