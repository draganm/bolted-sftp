Feature: get file

    Scenario: fetching existing file
        Given file with some database
        When I fetch the file
        Then I should get the content of the file