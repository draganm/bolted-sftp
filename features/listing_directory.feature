Feature: listing directories

    Scenario: listing empty directory
        Given an empty database
        When I list the root directory
        Then the result should be empty

    Scenario: listing directory with one entry
        Given a database with one map in the root
        When I list the root directory
        Then the result should have one directory
