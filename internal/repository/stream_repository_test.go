package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/suite"
)

type StreamRepositoryTestSuite struct {
	suite.Suite
	repo StreamRepository
}

func (s *StreamRepositoryTestSuite) SetupTest() {
	// Реализация конкретного репозитория
}

func TestStreamRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(StreamRepositoryTestSuite))
}

func (suite *StreamRepositoryTestSuite) TestCreateAndGetByID() {

	ctx := context.Background()

	if suite.repo == nil {
		suite.T().Skip("Repository not implemented yet")
	}

	stream := &models.Stream{
		Title:       "Test stream",
		Description: "Test Description",
		OwnerID:     uuid.New(),
		Status:      models.StatusDraft,
		Visibility:  models.VisibilityPrivate,
	}

	err := suite.repo.Create(ctx, stream)
	suite.Require().NoError(err)
	suite.Require().NotEqual(uuid.Nil, stream.ID)

	retrieved, err := suite.repo.Read(ctx, stream.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(retrieved)

	suite.Require().Equal(stream.Title, retrieved.Title)
	suite.Require().Equal(stream.ID, retrieved.ID)
	suite.Require().Equal(stream.OwnerID, retrieved.OwnerID)
}

func (suite *StreamRepositoryTestSuite) TestUpdate() {
	ctx := context.Background()

	if suite.repo == nil {
		suite.T().Skip("Repository not implemented yet")
	}

	stream := suite.createTestStream(ctx)

	stream.Title = "Updated title"
	err := suite.repo.Update(ctx, stream)
	suite.Require().NoError(err)

	updated, err := suite.repo.Read(ctx, stream.ID)

	suite.Require().NoError(err)
	suite.Require().Equal(updated.Title, stream.Title)

}

func (suite *StreamRepositoryTestSuite) TestDelete() {
	ctx := context.Background()

	stream := suite.createTestStream(ctx)

	err := suite.repo.Delete(ctx, stream.ID)
	suite.Require().NoError(err)

	deleted, err := suite.repo.Read(ctx, stream.ID)
	suite.Require().Error(err)
	suite.Assert().Nil(deleted)
}

func (suite *StreamRepositoryTestSuite) TestListWithFilter() {
	ctx := context.Background()
	ownerID := uuid.New()

	stream1 := &models.Stream{
		Title:   "Stream 1",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	stream2 := &models.Stream{
		Title:   "Stream 2",
		OwnerID: ownerID,
		Status:  models.StatusPublished,
	}

	suite.Require().NoError(suite.repo.Create(ctx, stream1))
	suite.Require().NoError(suite.repo.Create(ctx, stream2))

	filter := StreamFilter{
		OwnerID: &ownerID,
		Limit:   10,
	}

	streams, err := suite.repo.List(ctx, filter)
	suite.Require().NoError(err)
	suite.Assert().Len(streams, 2)
}

func (suite *StreamRepositoryTestSuite) TestUpdateStatus() {
	ctx := context.Background()
	stream := suite.createTestStream(ctx)

	err := suite.repo.UpdateStatus(ctx, stream.ID, models.StatusPublished)
	suite.Assert().NoError(err)

	updated, err := suite.repo.Read(ctx, stream.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(models.StatusPublished, updated.Status)

}

func (suite *StreamRepositoryTestSuite) createTestStream(ctx context.Context) *models.Stream {
	stream := &models.Stream{
		Title:       "Test stream",
		Description: "Test Description",
		OwnerID:     uuid.New(),
		Status:      models.StatusDraft,
		Visibility:  models.VisibilityPrivate,
	}
	err := suite.repo.Create(ctx, stream)
	suite.Require().NoError(err)

	return stream
}
